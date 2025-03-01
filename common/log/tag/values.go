// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package tag

// Pre-defined values for TagWorkflowAction
var (
	// workflow start / finish
	WorkflowActionWorkflowStarted       = workflowAction("add-workflow-started-event")
	WorkflowActionWorkflowCanceled      = workflowAction("add-workflow-canceled-event")
	WorkflowActionWorkflowCompleted     = workflowAction("add-workflow-completed--event")
	WorkflowActionWorkflowFailed        = workflowAction("add-workflow-failed-event")
	WorkflowActionWorkflowTimeout       = workflowAction("add-workflow-timeout-event")
	WorkflowActionWorkflowTerminated    = workflowAction("add-workflow-terminated-event")
	WorkflowActionWorkflowContinueAsNew = workflowAction("add-workflow-continue-as-new-event")

	// workflow cancellation / sign
	WorkflowActionWorkflowCancelRequested = workflowAction("add-workflow-cancel-requested-event")
	WorkflowActionWorkflowSignaled        = workflowAction("add-workflow-signaled-event")
	WorkflowActionWorkflowRecordMarker    = workflowAction("add-workflow-marker-record-event")

	// decision
	WorkflowActionDecisionTaskScheduled = workflowAction("add-decisiontask-scheduled-event")
	WorkflowActionDecisionTaskStarted   = workflowAction("add-decisiontask-started-event")
	WorkflowActionDecisionTaskCompleted = workflowAction("add-decisiontask-completed-event")
	WorkflowActionDecisionTaskTimedOut  = workflowAction("add-decisiontask-timedout-event")
	WorkflowActionDecisionTaskFailed    = workflowAction("add-decisiontask-failed-event")

	// activity
	WorkflowActionActivityTaskScheduled       = workflowAction("add-activitytask-scheduled-event")
	WorkflowActionActivityTaskStarted         = workflowAction("add-activitytask-started-event")
	WorkflowActionActivityTaskCompleted       = workflowAction("add-activitytask-completed-event")
	WorkflowActionActivityTaskFailed          = workflowAction("add-activitytask-failed-event")
	WorkflowActionActivityTaskTimedOut        = workflowAction("add-activitytask-timed-event")
	WorkflowActionActivityTaskCanceled        = workflowAction("add-activitytask-canceled-event")
	WorkflowActionActivityTaskCancelRequested = workflowAction("add-activitytask-cancel-requested-event")
	WorkflowActionActivityTaskCancelFailed    = workflowAction("add-activitytask-cancel-failed-event")

	// timer
	WorkflowActionTimerStarted      = workflowAction("add-timer-started-event")
	WorkflowActionTimerFired        = workflowAction("add-timer-fired-event")
	WorkflowActionTimerCanceled     = workflowAction("add-timer-canceled-event")
	WorkflowActionTimerCancelFailed = workflowAction("add-timer-cancel-failed-event")

	// child workflow start / finish
	WorkflowActionChildWorkflowInitiated        = workflowAction("add-childworkflow-initiated-event")
	WorkflowActionChildWorkflowStarted          = workflowAction("add-childworkflow-started-event")
	WorkflowActionChildWorkflowInitiationFailed = workflowAction("add-childworkflow-initiation-failed-event")
	WorkflowActionChildWorkflowCanceled         = workflowAction("add-childworkflow-canceled-event")
	WorkflowActionChildWorkflowCompleted        = workflowAction("add-childworkflow-completed-event")
	WorkflowActionChildWorkflowFailed           = workflowAction("add-childworkflow-failed-event")
	WorkflowActionChildWorkflowTerminated       = workflowAction("add-childworkflow-terminated-event")
	WorkflowActionChildWorkflowTimedOut         = workflowAction("add-childworkflow-timedout-event")

	// external workflow cancellation
	WorkflowActionExternalWorkflowCancelInitiated = workflowAction("add-externalworkflow-cancel-initiated-event")
	WorkflowActionExternalWorkflowCancelRequested = workflowAction("add-externalworkflow-cancel-requested-event")
	WorkflowActionExternalWorkflowCancelFailed    = workflowAction("add-externalworkflow-cancel-failed-event")

	// external workflow signal
	WorkflowActionExternalWorkflowSignalInitiated = workflowAction("add-externalworkflow-signal-initiated-event")
	WorkflowActionExternalWorkflowSignalRequested = workflowAction("add-externalworkflow-signal-requested-event")
	WorkflowActionExternalWorkflowSignalFailed    = workflowAction("add-externalworkflow-signal-failed-event")

	WorkflowActionUnknown = workflowAction("add-unknown-event")
)

// Pre-defined values for TagWorkflowListFilterType
var (
	WorkflowListWorkflowFilterByID     = workflowListFilterType("WID")
	WorkflowListWorkflowFilterByType   = workflowListFilterType("WType")
	WorkflowListWorkflowFilterByStatus = workflowListFilterType("status")
)

// Pre-defined values for TagSysComponent
var (
	ComponentTaskList                 = component("tasklist")
	ComponentHistoryBuilder           = component("history-builder")
	ComponentHistoryEngine            = component("history-engine")
	ComponentHistoryCache             = component("history-cache")
	ComponentEventsCache              = component("events-cache")
	ComponentTransferQueue            = component("transfer-queue-processor")
	ComponentTimerQueue               = component("timer-queue-processor")
	ComponentTimerBuilder             = component("timer-builder")
	ComponentReplicatorQueue          = component("replicator-queue-processor")
	ComponentShardController          = component("shard-controller")
	ComponentShard                    = component("shard")
	ComponentShardItem                = component("shard-item")
	ComponentShardEngine              = component("shard-engine")
	ComponentMatchingEngine           = component("matching-engine")
	ComponentReplicator               = component("replicator")
	ComponentReplicationTaskProcessor = component("replication-task-processor")
	ComponentHistoryReplicator        = component("history-replicator")
	ComponentIndexer                  = component("indexer")
	ComponentIndexerProcessor         = component("indexer-processor")
	ComponentIndexerESProcessor       = component("indexer-es-processor")
	ComponentESVisibilityManager      = component("es-visibility-manager")
	ComponentArchiver                 = component("archiver")
	ComponentBatcher                  = component("batcher")
	ComponentWorker                   = component("worker")
	ComponentServiceResolver          = component("service-resolver")
)

// Pre-defined values for TagSysLifecycle
var (
	LifeCycleStarting         = lifecycle("Starting")
	LifeCycleStarted          = lifecycle("Started")
	LifeCycleStopping         = lifecycle("Stopping")
	LifeCycleStopped          = lifecycle("Stopped")
	LifeCycleStopTimedout     = lifecycle("StopTimedout")
	LifeCycleStartFailed      = lifecycle("StartFailed")
	LifeCycleStopFailed       = lifecycle("StopFailed")
	LifeCycleProcessingFailed = lifecycle("ProcessingFailed")
)

// Pre-defined values for SysErrorType
var (
	ErrorTypeInvalidHistoryAction        = errorType("InvalidHistoryAction")
	ErrorTypeInvalidQueryTask            = errorType("InvalidQueryTask")
	ErrorTypeQueryTaskFailed             = errorType("QueryTaskFailed")
	ErrorTypePersistentStoreError        = errorType("PersistentStoreError")
	ErrorTypeHistorySerializationError   = errorType("HistorySerializationError")
	ErrorTypeHistoryDeserializationError = errorType("HistoryDeserializationError")
	ErrorTypeDuplicateTask               = errorType("DuplicateTask")
	ErrorTypeMultipleCompletionDecisions = errorType("MultipleCompletionDecisions")
	ErrorTypeDuplicateTransferTask       = errorType("DuplicateTransferTask")
	ErrorTypeDecisionFailed              = errorType("DecisionFailed")
	ErrorTypeInvalidMutableStateAction   = errorType("InvalidMutableStateAction")
)

// Pre-defined values for SysShardUpdate
var (
	// Shard context events
	ValueShardRangeUpdated            = shardupdate("ShardRangeUpdated")
	ValueShardAllocateTimerBeforeRead = shardupdate("ShardAllocateTimerBeforeRead")
	ValueRingMembershipChangedEvent   = shardupdate("RingMembershipChangedEvent")
)

// Pre-defined values for OperationResult
var (
	OperationFailed   = operationResult("OperationFailed")
	OperationStuck    = operationResult("OperationStuck")
	OperationCritical = operationResult("OperationCritical")
)

// Pre-defined values for TagSysStoreOperation
var (
	StoreOperationGetTasks                = storeOperation("get-tasks")
	StoreOperationCompleteTask            = storeOperation("complete-task")
	StoreOperationCompleteTasksLessThan   = storeOperation("complete-tasks-less-than")
	StoreOperationCreateWorkflowExecution = storeOperation("create-wf-execution")
	StoreOperationGetWorkflowExecution    = storeOperation("get-wf-execution")
	StoreOperationUpdateWorkflowExecution = storeOperation("update-wf-execution")
	StoreOperationDeleteWorkflowExecution = storeOperation("delete-wf-execution")
	StoreOperationUpdateShard             = storeOperation("update-shard")
	StoreOperationCreateTask              = storeOperation("create-task")
	StoreOperationUpdateTaskList          = storeOperation("update-task-list")
	StoreOperationStopTaskList            = storeOperation("stop-task-list")
)
