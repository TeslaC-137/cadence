package persistence

import (
	"math"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/gocql/gocql"

	workflow "code.uber.internal/devexp/minions/.gen/go/shared"
	"code.uber.internal/devexp/minions/common"
	"code.uber.internal/devexp/minions/common/logging"
)

const (
	testWorkflowClusterHosts = "127.0.0.1"
	testSchemaDir            = "../.."
)

type (
	// TestBaseOptions options to configure workflow test base.
	TestBaseOptions struct {
		ClusterHost  string
		KeySpace     string
		DropKeySpace bool
		SchemaDir    string
	}

	// TestBase wraps the base setup needed to create workflows over engine layer.
	TestBase struct {
		ShardMgr     ShardManager
		WorkflowMgr  ExecutionManager
		TaskMgr      TaskManager
		ShardInfo    *ShardInfo
		ShardContext *testShardContext
		readLevel    int64
		CassandraTestCluster
	}

	// CassandraTestCluster allows executing cassandra operations in testing.
	CassandraTestCluster struct {
		keyspace string
		cluster  *gocql.ClusterConfig
		session  *gocql.Session
	}

	testShardContext struct {
		shardInfo              *ShardInfo
		transferSequenceNumber int64
		timerSequeceNumber     int64
	}
)

func newTestShardContext(shardInfo *ShardInfo, transferSequenceNumber int64) *testShardContext {
	return &testShardContext{
		shardInfo:              shardInfo,
		transferSequenceNumber: transferSequenceNumber,
	}
}

func (s *testShardContext) GetTransferTaskID() int64 {
	return atomic.AddInt64(&s.transferSequenceNumber, 1)
}

func (s *testShardContext) GetRangeID() int64 {
	return atomic.LoadInt64(&s.shardInfo.RangeID)
}

func (s *testShardContext) GetTransferAckLevel() int64 {
	return atomic.LoadInt64(&s.shardInfo.TransferAckLevel)
}

func (s *testShardContext) GetTimerSequenceNumber() int64 {
	return atomic.AddInt64(&s.timerSequeceNumber, 1)
}

func (s *testShardContext) UpdateAckLevel(ackLevel int64) error {
	atomic.StoreInt64(&s.shardInfo.TransferAckLevel, ackLevel)
	return nil
}

func (s *testShardContext) GetTransferSequenceNumber() int64 {
	return atomic.LoadInt64(&s.transferSequenceNumber)
}

func (s *testShardContext) Reset() {
	atomic.StoreInt64(&s.shardInfo.RangeID, 0)
	atomic.StoreInt64(&s.shardInfo.TransferAckLevel, 0)
}

// SetupWorkflowStoreWithOptions to setup workflow test base
func (s *TestBase) SetupWorkflowStoreWithOptions(options TestBaseOptions) {
	// Setup Workflow keyspace and deploy schema for tests
	s.CassandraTestCluster.setupTestCluster(options.KeySpace, options.DropKeySpace, options.SchemaDir)
	shardID := 0
	var err error
	s.ShardMgr, err = NewCassandraShardPersistence(options.ClusterHost, s.CassandraTestCluster.keyspace)
	if err != nil {
		log.Fatal(err)
	}
	s.WorkflowMgr, err = NewCassandraWorkflowExecutionPersistence(options.ClusterHost,
		s.CassandraTestCluster.keyspace, shardID)
	if err != nil {
		log.Fatal(err)
	}
	s.TaskMgr, err = NewCassandraTaskPersistence(options.ClusterHost, s.CassandraTestCluster.keyspace)
	if err != nil {
		log.Fatal(err)
	}
	// Create a shard for test
	s.readLevel = 0
	s.ShardInfo = &ShardInfo{
		ShardID:          shardID,
		RangeID:          0,
		TransferAckLevel: 0,
	}
	s.ShardContext = newTestShardContext(s.ShardInfo, 0)
	err1 := s.ShardMgr.CreateShard(&CreateShardRequest{
		ShardInfo: s.ShardInfo,
	})
	if err1 != nil {
		log.Fatal(err1)
	}
}

// CreateShard is a utility method to create the shard using persistence layer
func (s *TestBase) CreateShard(shardID int, owner string, rangeID int64) error {
	info := &ShardInfo{
		ShardID: shardID,
		Owner:   owner,
		RangeID: rangeID,
	}

	return s.ShardMgr.CreateShard(&CreateShardRequest{
		ShardInfo: info,
	})
}

// GetShard is a utility method to get the shard using persistence layer
func (s *TestBase) GetShard(shardID int) (*ShardInfo, error) {
	response, err := s.ShardMgr.GetShard(&GetShardRequest{
		ShardID: shardID,
	})

	if err != nil {
		return nil, err
	}

	return response.ShardInfo, nil
}

// UpdateShard is a utility method to update the shard using persistence layer
func (s *TestBase) UpdateShard(updatedInfo *ShardInfo, previousRangeID int64) error {
	return s.ShardMgr.UpdateShard(&UpdateShardRequest{
		ShardInfo:       updatedInfo,
		PreviousRangeID: previousRangeID,
	})
}

// CreateWorkflowExecution is a utility method to create workflow executions
func (s *TestBase) CreateWorkflowExecution(workflowExecution workflow.WorkflowExecution, taskList string,
	history string, executionContext []byte, nextEventID int64, lastProcessedEventID int64, decisionScheduleID int64,
	timerTasks []Task) (
	string, error) {
	response, err := s.WorkflowMgr.CreateWorkflowExecution(&CreateWorkflowExecutionRequest{
		Execution:          workflowExecution,
		TaskList:           taskList,
		History:            []byte(history),
		ExecutionContext:   executionContext,
		NextEventID:        nextEventID,
		LastProcessedEvent: lastProcessedEventID,
		RangeID:            s.ShardContext.GetRangeID(),
		TransferTasks: []Task{
			&DecisionTask{TaskID: s.GetNextSequenceNumber(), TaskList: taskList, ScheduleID: decisionScheduleID},
		},
		TimerTasks: timerTasks})

	if err != nil {
		return "", err
	}

	return response.TaskID, nil
}

// CreateWorkflowExecutionManyTasks is a utility method to create workflow executions
func (s *TestBase) CreateWorkflowExecutionManyTasks(workflowExecution workflow.WorkflowExecution,
	taskList string, history string, executionContext []byte, nextEventID int64, lastProcessedEventID int64,
	decisionScheduleIDs []int64, activityScheduleIDs []int64) (string, error) {

	transferTasks := []Task{}
	for _, decisionScheduleID := range decisionScheduleIDs {
		transferTasks = append(transferTasks,
			&DecisionTask{TaskID: s.GetNextSequenceNumber(), TaskList: taskList, ScheduleID: int64(decisionScheduleID)})
	}

	for _, activityScheduleID := range activityScheduleIDs {
		transferTasks = append(transferTasks,
			&ActivityTask{TaskID: s.GetNextSequenceNumber(), TaskList: taskList, ScheduleID: int64(activityScheduleID)})
	}

	response, err := s.WorkflowMgr.CreateWorkflowExecution(&CreateWorkflowExecutionRequest{
		Execution:          workflowExecution,
		TaskList:           taskList,
		History:            []byte(history),
		ExecutionContext:   executionContext,
		NextEventID:        nextEventID,
		LastProcessedEvent: lastProcessedEventID,
		TransferTasks:      transferTasks,
		RangeID:            s.ShardContext.GetRangeID()})

	if err != nil {
		return "", err
	}

	return response.TaskID, nil
}

// GetWorkflowExecutionInfo is a utility method to retrieve execution info
func (s *TestBase) GetWorkflowExecutionInfo(workflowExecution workflow.WorkflowExecution) (*WorkflowExecutionInfo,
	error) {
	response, err := s.WorkflowMgr.GetWorkflowExecution(&GetWorkflowExecutionRequest{
		Execution: workflowExecution,
	})
	if err != nil {
		return nil, err
	}

	return response.ExecutionInfo, nil
}

// GetWorkflowMutableState is a utility method to retrieve execution info
func (s *TestBase) GetWorkflowMutableState(workflowExecution workflow.WorkflowExecution) (*WorkflowMutableState,
	error) {
	response, err := s.WorkflowMgr.GetWorkflowMutableState(&GetWorkflowMutableStateRequest{
		WorkflowID: workflowExecution.GetWorkflowId(),
		RunID:      workflowExecution.GetRunId(),
	})
	if err != nil {
		return nil, err
	}

	return response.State, nil
}

// UpdateWorkflowExecution is a utility method to update workflow execution
func (s *TestBase) UpdateWorkflowExecution(updatedInfo *WorkflowExecutionInfo, decisionScheduleIDs []int64,
	activityScheduleIDs []int64, condition int64, timerTasks []Task, deleteTimerTask Task,
	upsertActivityInfos []*ActivityInfo, deleteActivityInfo *int64,
	upsertTimerInfos []*TimerInfo, deleteTimerInfos []string) error {
	transferTasks := []Task{}
	for _, decisionScheduleID := range decisionScheduleIDs {
		transferTasks = append(transferTasks, &DecisionTask{TaskList: updatedInfo.TaskList,
			ScheduleID: int64(decisionScheduleID)})
	}

	for _, activityScheduleID := range activityScheduleIDs {
		transferTasks = append(transferTasks, &ActivityTask{TaskList: updatedInfo.TaskList,
			ScheduleID: int64(activityScheduleID)})
	}

	return s.WorkflowMgr.UpdateWorkflowExecution(&UpdateWorkflowExecutionRequest{
		ExecutionInfo:       updatedInfo,
		TransferTasks:       transferTasks,
		TimerTasks:          timerTasks,
		Condition:           condition,
		DeleteTimerTask:     deleteTimerTask,
		RangeID:             s.ShardContext.GetRangeID(),
		UpsertActivityInfos: upsertActivityInfos,
		DeleteActivityInfo:  deleteActivityInfo,
		UpserTimerInfos:     upsertTimerInfos,
		DeleteTimerInfos:    deleteTimerInfos,
	})
}

// DeleteWorkflowExecution is a utility method to delete a workflow execution
func (s *TestBase) DeleteWorkflowExecution(info *WorkflowExecutionInfo) error {
	return s.WorkflowMgr.DeleteWorkflowExecution(&DeleteWorkflowExecutionRequest{
		ExecutionInfo: info,
	})
}

// GetTransferTasks is a utility method to get tasks from transfer task queue
func (s *TestBase) GetTransferTasks(batchSize int) ([]*TransferTaskInfo, error) {
	response, err := s.WorkflowMgr.GetTransferTasks(&GetTransferTasksRequest{
		ReadLevel:    s.GetReadLevel(),
		MaxReadLevel: s.GetMaxAllowedReadLevel(),
		BatchSize:    batchSize,
		RangeID:      s.ShardContext.GetRangeID(),
	})

	if err != nil {
		return nil, err
	}

	for _, task := range response.Tasks {
		atomic.StoreInt64(&s.readLevel, task.TaskID)
	}

	return response.Tasks, nil
}

// CompleteTransferTask is a utility method to complete a transfer task
func (s *TestBase) CompleteTransferTask(workflowExecution workflow.WorkflowExecution, taskID int64) error {

	return s.WorkflowMgr.CompleteTransferTask(&CompleteTransferTaskRequest{
		Execution: workflowExecution,
		TaskID:    taskID,
	})
}

// GetTimerIndexTasks is a utility method to get tasks from transfer task queue
func (s *TestBase) GetTimerIndexTasks(minKey int64, maxKey int64) ([]*TimerTaskInfo, error) {
	response, err := s.WorkflowMgr.GetTimerIndexTasks(&GetTimerIndexTasksRequest{
		MinKey: minKey, MaxKey: maxKey, BatchSize: 10})

	if err != nil {
		return nil, err
	}

	return response.Timers, nil
}

// CreateDecisionTask is a utility method to create a task
func (s *TestBase) CreateDecisionTask(workflowExecution workflow.WorkflowExecution, taskList string,
	decisionScheduleID int64) (int64, error) {
	leaseResponse, err := s.TaskMgr.LeaseTaskList(&LeaseTaskListRequest{TaskList: taskList, TaskType: TaskTypeDecision})
	if err != nil {
		return 0, err
	}

	taskID := s.GetNextSequenceNumber()
	_, err = s.TaskMgr.CreateTask(&CreateTaskRequest{
		TaskID:    taskID,
		Execution: workflowExecution,
		Data: &DecisionTask{
			TaskID:     taskID,
			TaskList:   taskList,
			ScheduleID: decisionScheduleID,
		},
		RangeID: leaseResponse.RangeID,
	})

	if err != nil {
		return 0, err
	}

	return taskID, err
}

// CreateActivityTasks is a utility method to create tasks
func (s *TestBase) CreateActivityTasks(workflowExecution workflow.WorkflowExecution, activities map[int64]string) (
	[]int64, error) {

	var taskIDs []int64
	var leaseResponse *LeaseTaskListResponse
	var err error
	for activityScheduleID, taskList := range activities {

		leaseResponse, err = s.TaskMgr.LeaseTaskList(
			&LeaseTaskListRequest{TaskList: taskList, TaskType: TaskTypeActivity})
		if err != nil {
			return []int64{}, err
		}
		taskID := s.GetNextSequenceNumber()

		_, err := s.TaskMgr.CreateTask(&CreateTaskRequest{
			TaskID:    taskID,
			Execution: workflowExecution,
			Data: &ActivityTask{
				TaskID:     s.GetNextSequenceNumber(),
				TaskList:   taskList,
				ScheduleID: activityScheduleID,
			},
			RangeID: leaseResponse.RangeID,
		})

		if err != nil {
			return nil, err
		}

		taskIDs = append(taskIDs, taskID)
	}

	return taskIDs, nil
}

// GetTasks is a utility method to get tasks from persistence
func (s *TestBase) GetTasks(taskList string, taskType int, batchSize int) (*GetTasksResponse, error) {
	leaseResponse, err := s.TaskMgr.LeaseTaskList(&LeaseTaskListRequest{TaskList: taskList, TaskType: taskType})
	if err != nil {
		return nil, err
	}

	response, err := s.TaskMgr.GetTasks(&GetTasksRequest{
		TaskList:     taskList,
		TaskType:     taskType,
		BatchSize:    batchSize,
		RangeID:      leaseResponse.RangeID,
		MaxReadLevel: math.MaxInt64,
	})

	if err != nil {
		return nil, err
	}

	return &GetTasksResponse{Tasks: response.Tasks}, nil
}

// CompleteTask is a utility method to complete a task
func (s *TestBase) CompleteTask(taskList string, taskType int, taskID int64) error {

	return s.TaskMgr.CompleteTask(&CompleteTaskRequest{
		TaskList: taskList,
		TaskType: taskType,
		TaskID:   taskID,
	})
}

// ClearTransferQueue completes all tasks in transfer queue
func (s *TestBase) ClearTransferQueue() {
	log.Infof("Clearing transfer tasks (RangeID: %v, ReadLevel: %v, AckLevel: %v)", s.ShardContext.GetRangeID(),
		s.GetReadLevel(), s.ShardContext.GetTransferAckLevel())
	tasks, err := s.GetTransferTasks(100)
	if err != nil {
		log.Fatalf("Error during cleanup: %v", err)
	}

	counter := 0
	for _, t := range tasks {
		log.Infof("Deleting transfer task with ID: %v", t.TaskID)
		e := workflow.WorkflowExecution{WorkflowId: common.StringPtr(t.WorkflowID), RunId: common.StringPtr(t.RunID)}
		s.CompleteTransferTask(e, t.TaskID)
		counter++
	}

	log.Infof("Deleted '%v' transfer tasks.", counter)
	s.ShardContext.Reset()
	atomic.StoreInt64(&s.readLevel, 0)
}

// SetupWorkflowStore to setup workflow test base
func (s *TestBase) SetupWorkflowStore() {
	s.SetupWorkflowStoreWithOptions(TestBaseOptions{SchemaDir: testSchemaDir, ClusterHost: testWorkflowClusterHosts, DropKeySpace: true})
}

// TearDownWorkflowStore to cleanup
func (s *TestBase) TearDownWorkflowStore() {
	s.CassandraTestCluster.tearDownTestCluster()
}

// GetNextSequenceNumber generates a unique sequence number for can be used for transfer queue taskId
func (s *TestBase) GetNextSequenceNumber() int64 {
	return s.ShardContext.GetTransferTaskID()
}

// GetReadLevel returns the current read level for shard
func (s *TestBase) GetReadLevel() int64 {
	return atomic.LoadInt64(&s.readLevel)
}

// GetMaxAllowedReadLevel returns the maximum allowed read level for the shard
func (s *TestBase) GetMaxAllowedReadLevel() int64 {
	return s.ShardContext.GetTransferSequenceNumber()
}

func (s *CassandraTestCluster) setupTestCluster(keySpace string, dropKeySpace bool, schemaDir string) {
	if keySpace == "" {
		keySpace = generateRandomKeyspace(10)
	}
	s.createCluster(testWorkflowClusterHosts, gocql.Consistency(1), keySpace)
	s.createKeyspace(1, dropKeySpace)
	s.loadSchema("workflow_test.cql", schemaDir)
}

func (s *CassandraTestCluster) tearDownTestCluster() {
	s.dropKeyspace()
	s.session.Close()
}

func (s *CassandraTestCluster) createCluster(clusterHosts string, cons gocql.Consistency, keyspace string) {
	s.cluster = common.NewCassandraCluster(clusterHosts)
	s.cluster.Consistency = cons
	s.cluster.Keyspace = "system"
	s.cluster.Timeout = 40 * time.Second
	var err error
	s.session, err = s.cluster.CreateSession()
	if err != nil {
		log.WithField(logging.TagErr, err).Fatal(`createSession`)
	}
	s.keyspace = keyspace
}

func (s *CassandraTestCluster) createKeyspace(replicas int, dropKeySpace bool) {
	err := common.CreateCassandraKeyspace(s.session, s.keyspace, replicas, dropKeySpace)
	if err != nil {
		log.Fatal(err)
	}

	s.cluster.Keyspace = s.keyspace
}

func (s *CassandraTestCluster) dropKeyspace() {
	err := common.DropCassandraKeyspace(s.session, s.keyspace)
	if err != nil && !strings.Contains(err.Error(), "AlreadyExists") {
		log.Fatal(err)
	}
}

func (s *CassandraTestCluster) loadSchema(fileName string, schemaDir string) {
	cqlshDir := "./cassandra/bin/cqlsh"
	workflowSchemaDir := "./schema/"

	if schemaDir != "" {
		cqlshDir = schemaDir + "/cassandra/bin/cqlsh"
		log.Error(cqlshDir)
		workflowSchemaDir = schemaDir + "/schema/"
	}

	err := common.LoadCassandraSchema(cqlshDir, workflowSchemaDir+fileName, s.keyspace)

	if err != nil && !strings.Contains(err.Error(), "AlreadyExists") {
		err = common.LoadCassandraSchema(cqlshDir, workflowSchemaDir+fileName, s.keyspace)
	}

	if err != nil && !strings.Contains(err.Error(), "AlreadyExists") {
		log.Fatal(err)
	}
}

func validateTimeRange(t time.Time, expectedDuration time.Duration) bool {
	currentTime := time.Now()
	diff := time.Duration(currentTime.UnixNano() - t.UnixNano())
	if diff > expectedDuration {
		log.Infof("Current time: %v, Application time: %v, Differenrce: %v", currentTime, t, diff)
		return false
	}
	return true
}

func generateRandomKeyspace(n int) string {
	rand.Seed(time.Now().UnixNano())
	letterRunes := []rune("workflow")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}