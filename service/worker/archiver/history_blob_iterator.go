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

package archiver

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/uber/cadence/.gen/go/shared"
	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/persistence"
)

/**
IMPORTANT: Under the assumption that history is immutable, the following iterator will deterministically return identical
blobs from Next regardless of the current cluster and regardless of the number of times this iteration occurs. This property
makes any concurrent uploads of history safe.
*/

type (
	// HistoryBlobIterator is used to get history blobs
	HistoryBlobIterator interface {
		Next() (*HistoryBlob, error)
		HasNext() bool
		GetState() ([]byte, error)
	}

	historyBlobIteratorState struct {
		BlobPageToken        int
		PersistencePageToken []byte
		FinishedIteration    bool
		NumEventsToSkip      int
	}

	historyBlobIterator struct {
		historyBlobIteratorState

		// the following are only used to read history and dynamic config
		historyManager       persistence.HistoryManager
		historyV2Manager     persistence.HistoryV2Manager
		domainID             string
		workflowID           string
		runID                string
		eventStoreVersion    int32
		branchToken          []byte
		nextEventID          int64
		config               *Config
		domain               string
		clusterName          string
		closeFailoverVersion int64
		shardID              int
		sizeEstimator        SizeEstimator
	}
)

var (
	errIteratorDepleted = errors.New("iterator is depleted")
)

// NewHistoryBlobIterator returns a new HistoryBlobIterator
func NewHistoryBlobIterator(
	request ArchiveRequest,
	container *BootstrapContainer,
	domainName string,
	clusterName string,
	initialState []byte,
) (HistoryBlobIterator, error) {
	it := &historyBlobIterator{
		historyBlobIteratorState: historyBlobIteratorState{
			BlobPageToken:        common.FirstBlobPageToken,
			PersistencePageToken: nil,
			FinishedIteration:    false,
			NumEventsToSkip:      0,
		},
		historyManager:       container.HistoryManager,
		historyV2Manager:     container.HistoryV2Manager,
		domainID:             request.DomainID,
		workflowID:           request.WorkflowID,
		runID:                request.RunID,
		eventStoreVersion:    request.EventStoreVersion,
		branchToken:          request.BranchToken,
		nextEventID:          request.NextEventID,
		config:               container.Config,
		domain:               domainName,
		clusterName:          clusterName,
		closeFailoverVersion: request.CloseFailoverVersion,
		shardID:              request.ShardID,
		sizeEstimator:        container.HistorySizeEstimator,
	}
	if it.sizeEstimator == nil {
		it.sizeEstimator = NewJSONSizeEstimator()
	}
	if initialState == nil {
		return it, nil
	}
	if err := it.reset(initialState); err != nil {
		return nil, err
	}
	return it, nil
}

// Next returns historyBlob and advances iterator.
// Returns error if iterator is empty, or if history could not be read.
// If error is returned then no iterator is state is advanced.
func (i *historyBlobIterator) Next() (*HistoryBlob, error) {
	if !i.HasNext() {
		return nil, errIteratorDepleted
	}
	events, newIterState, err := i.readBlobEvents(i.PersistencePageToken, i.NumEventsToSkip)
	if err != nil {
		return nil, err
	}

	// only if no error was encountered reading history does the state of the iterator get advanced
	i.FinishedIteration = newIterState.FinishedIteration
	i.PersistencePageToken = newIterState.PersistencePageToken
	i.NumEventsToSkip = newIterState.NumEventsToSkip

	firstEvent := events[0]
	lastEvent := events[len(events)-1]
	eventCount := int64(len(events))
	header := &HistoryBlobHeader{
		DomainName:           &i.domain,
		DomainID:             &i.domainID,
		WorkflowID:           &i.workflowID,
		RunID:                &i.runID,
		CurrentPageToken:     common.IntPtr(i.BlobPageToken),
		NextPageToken:        common.IntPtr(common.LastBlobNextPageToken),
		IsLast:               common.BoolPtr(true),
		FirstFailoverVersion: firstEvent.Version,
		LastFailoverVersion:  lastEvent.Version,
		FirstEventID:         firstEvent.EventId,
		LastEventID:          lastEvent.EventId,
		UploadDateTime:       common.StringPtr(time.Now().String()),
		UploadCluster:        &i.clusterName,
		EventCount:           &eventCount,
		CloseFailoverVersion: &i.closeFailoverVersion,
	}
	if i.HasNext() {
		i.BlobPageToken++
		header.NextPageToken = common.IntPtr(i.BlobPageToken)
		header.IsLast = common.BoolPtr(false)
	}
	return &HistoryBlob{
		Header: header,
		Body: &shared.History{
			Events: events,
		},
	}, nil
}

// HasNext returns true if there are more items to iterate over.
func (i *historyBlobIterator) HasNext() bool {
	return !i.FinishedIteration
}

// GetState returns the encoded iterator state
func (i *historyBlobIterator) GetState() ([]byte, error) {
	return json.Marshal(i.historyBlobIteratorState)
}

// readBlobEvents gets history events, starting from page identified by given pageToken.
// Reads events until all of history has been read or enough events have been fetched to satisfy blob size target.
// If empty pageToken is given, then iteration will start from the beginning of history.
// Does not modify any iterator state (i.e. calls to readBlobEvents are idempotent).
// Returns the following four things:
// 1. HistoryEvents: Either all of history starting from given pageToken or enough history to satisfy blob size target.
// 2. NewIteratorState: Including persistence page token, finished iteration and numEventsToSkip
// 3. Error: Any error that occurred while reading history
func (i *historyBlobIterator) readBlobEvents(pageToken []byte, numEventsToSkip int) ([]*shared.HistoryEvent, historyBlobIteratorState, error) {
	currSize := 0
	targetSize := i.config.TargetArchivalBlobSize(i.domain)
	var historyEvents []*shared.HistoryEvent
	newIterState := historyBlobIteratorState{}
	for currSize == 0 || (len(pageToken) > 0 && currSize < targetSize) {
		currHistoryEvents, nextPageToken, err := i.readHistory(pageToken)
		if err != nil {
			return nil, newIterState, err
		}
		currHistoryEvents = currHistoryEvents[numEventsToSkip:]
		for idx, event := range currHistoryEvents {
			eventSize, err := i.sizeEstimator.EstimateSize(event)
			if err != nil {
				return nil, newIterState, err
			}
			currSize += eventSize
			historyEvents = append(historyEvents, event)

			// If targetSize is meeted after appending the last event, we are not sure if there's more events or not,
			// so we need to exclude that case.
			if currSize >= targetSize && idx != len(currHistoryEvents)-1 {
				newIterState.PersistencePageToken = pageToken
				newIterState.NumEventsToSkip = numEventsToSkip + idx + 1
				return historyEvents, newIterState, nil
			}
		}
		numEventsToSkip = 0
		pageToken = nextPageToken
	}

	if len(pageToken) == 0 {
		newIterState.FinishedIteration = true
		return historyEvents, newIterState, nil
	}

	// If nextPageToken is not empty it is still possible there are no more events.
	// This occurs if history was read exactly to the last event.
	// Here we look forward one page so that we can treat reading exactly to the end of history
	// the same way as reading through the end of history.
	lookAheadHistoryEvents, _, err := i.readHistory(pageToken)
	if err != nil {
		return nil, newIterState, err
	}
	if len(lookAheadHistoryEvents) == 0 {
		newIterState.FinishedIteration = true
		return historyEvents, newIterState, nil
	}
	newIterState.PersistencePageToken = pageToken
	return historyEvents, newIterState, nil
}

// readHistory fetches a single page of history events identified by given pageToken.
// Does not modify any iterator state (i.e. calls to readHistory are idempotent).
// Returns historyEvents, nextPageToken and error.
func (i *historyBlobIterator) readHistory(pageToken []byte) ([]*shared.HistoryEvent, []byte, error) {
	if i.eventStoreVersion == persistence.EventStoreVersionV2 {
		req := &persistence.ReadHistoryBranchRequest{
			BranchToken:   i.branchToken,
			MinEventID:    common.FirstEventID,
			MaxEventID:    i.nextEventID,
			PageSize:      i.config.HistoryPageSize(i.domain),
			NextPageToken: pageToken,
			ShardID:       common.IntPtr(i.shardID),
		}
		historyEvents, _, nextPageToken, err := persistence.ReadFullPageV2Events(i.historyV2Manager, req)
		return historyEvents, nextPageToken, err
	}
	req := &persistence.GetWorkflowExecutionHistoryRequest{
		DomainID: i.domainID,
		Execution: shared.WorkflowExecution{
			WorkflowId: common.StringPtr(i.workflowID),
			RunId:      common.StringPtr(i.runID),
		},
		FirstEventID:  common.FirstEventID,
		NextEventID:   i.nextEventID,
		PageSize:      i.config.HistoryPageSize(i.domain),
		NextPageToken: pageToken,
	}
	resp, err := i.historyManager.GetWorkflowExecutionHistory(req)
	if err != nil {
		return nil, nil, err
	}
	return resp.History.Events, resp.NextPageToken, nil
}

type (
	// SizeEstimator is used to estimate the size of any object
	SizeEstimator interface {
		EstimateSize(v interface{}) (int, error)
	}

	jsonSizeEstimator struct{}
)

func (e *jsonSizeEstimator) EstimateSize(v interface{}) (int, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

// NewJSONSizeEstimator returns a new SizeEstimator which uses json encoding to
// estimate size
func NewJSONSizeEstimator() SizeEstimator {
	return &jsonSizeEstimator{}
}

// reset resets iterator to a certain state given its encoded representation
// if it returns an error, the operation will have no effect on the iterator
func (i *historyBlobIterator) reset(stateToken []byte) error {
	var iteratorState historyBlobIteratorState
	if err := json.Unmarshal(stateToken, &iteratorState); err != nil {
		return err
	}
	i.historyBlobIteratorState = iteratorState
	return nil
}
