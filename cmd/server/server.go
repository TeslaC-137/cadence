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

package main

import (
	"log"
	"time"

	"github.com/uber/cadence/common/cluster"

	"github.com/uber/cadence/client"
	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/blobstore/filestore"
	"github.com/uber/cadence/common/blobstore/s3store"
	"github.com/uber/cadence/common/elasticsearch"
	"github.com/uber/cadence/common/log/loggerimpl"
	"github.com/uber/cadence/common/log/tag"
	"github.com/uber/cadence/common/messaging"
	"github.com/uber/cadence/common/metrics"
	"github.com/uber/cadence/common/service"
	"github.com/uber/cadence/common/service/config"
	"github.com/uber/cadence/common/service/dynamicconfig"
	"github.com/uber/cadence/service/frontend"
	"github.com/uber/cadence/service/history"
	"github.com/uber/cadence/service/matching"
	"github.com/uber/cadence/service/worker"
	"go.uber.org/cadence/.gen/go/cadence/workflowserviceclient"
	"go.uber.org/zap"
)

type (
	server struct {
		name   string
		cfg    *config.Config
		doneC  chan struct{}
		daemon common.Daemon
	}
)

const (
	frontendService = "frontend"
	historyService  = "history"
	matchingService = "matching"
	workerService   = "worker"
)

// newServer returns a new instance of a daemon
// that represents a cadence service
func newServer(service string, cfg *config.Config) common.Daemon {
	return &server{
		cfg:   cfg,
		name:  service,
		doneC: make(chan struct{}),
	}
}

// Start starts the server
func (s *server) Start() {
	if _, ok := s.cfg.Services[s.name]; !ok {
		log.Fatalf("`%v` service missing config", s)
	}
	s.daemon = s.startService()
}

// Stop stops the server
func (s *server) Stop() {

	if s.daemon == nil {
		return
	}

	select {
	case <-s.doneC:
	default:
		s.daemon.Stop()
		select {
		case <-s.doneC:
		case <-time.After(time.Minute):
			log.Printf("timed out waiting for server %v to exit\n", s.name)
		}
	}
}

// startService starts a service with the given name and config
func (s *server) startService() common.Daemon {

	var err error

	params := service.BootstrapParams{}
	params.Name = "cadence-" + s.name
	params.Logger = loggerimpl.NewLogger(s.cfg.Log.NewZapLogger())
	params.PersistenceConfig = s.cfg.Persistence

	params.MembershipFactory, err = s.cfg.Ringpop.NewFactory(params.Logger, params.Name)
	if err != nil {
		log.Fatalf("error creating ringpop factory: %v", err)
	}

	params.DynamicConfig, err = dynamicconfig.NewFileBasedClient(&s.cfg.DynamicConfigClient, params.Logger.WithTags(tag.Service(params.Name)), s.doneC)
	if err != nil {
		log.Printf("error creating file based dynamic config client, use no-op config client instead. error: %v", err)
		params.DynamicConfig = dynamicconfig.NewNopClient()
	}
	dc := dynamicconfig.NewCollection(params.DynamicConfig, params.Logger)

	svcCfg := s.cfg.Services[s.name]
	params.MetricScope = svcCfg.Metrics.NewScope(params.Logger)
	params.RPCFactory = svcCfg.RPC.NewFactory(params.Name, params.Logger)
	params.PProfInitializer = svcCfg.PProf.NewInitializer(params.Logger)

	archivalStatus := dc.GetStringProperty(dynamicconfig.ArchivalStatus, s.cfg.Archival.Status)
	enableReadFromArchival := dc.GetBoolProperty(dynamicconfig.EnableReadFromArchival, s.cfg.Archival.EnableReadFromArchival)

	params.DCRedirectionPolicy = s.cfg.DCRedirectionPolicy

	params.MetricsClient = metrics.NewClient(params.MetricScope, service.GetMetricsServiceIdx(params.Name, params.Logger))

	clusterMetadata := s.cfg.ClusterMetadata
	// TODO remove when ClustersInfo is fully deprecated
	if len(s.cfg.ClustersInfo.CurrentClusterName) != 0 && len(s.cfg.ClusterMetadata.CurrentClusterName) != 0 {
		log.Fatalf("cannot config both clustersInfo and clusterMetadata")
	}
	if len(s.cfg.ClustersInfo.CurrentClusterName) != 0 {
		clusterMetadata = s.cfg.ClustersInfo.ToClusterMetadata()
	}
	params.ClusterMetadata = cluster.NewMetadata(
		params.Logger,
		dc.GetBoolProperty(dynamicconfig.EnableGlobalDomain, clusterMetadata.EnableGlobalDomain),
		clusterMetadata.FailoverVersionIncrement,
		clusterMetadata.MasterClusterName,
		clusterMetadata.CurrentClusterName,
		clusterMetadata.ClusterInformation,
		archivalStatus(),
		s.cfg.Archival.DefaultBucket,
		enableReadFromArchival(),
	)
	params.DispatcherProvider = client.NewIPYarpcDispatcherProvider()
	params.ESConfig = &s.cfg.ElasticSearch
	params.ESConfig.Enable = dc.GetBoolProperty(dynamicconfig.EnableVisibilityToKafka, params.ESConfig.Enable)() // force override with dynamic config
	if params.ClusterMetadata.IsGlobalDomainEnabled() {
		params.MessagingClient = messaging.NewKafkaClient(&s.cfg.Kafka, params.MetricsClient, zap.NewNop(), params.Logger, params.MetricScope, true, params.ESConfig.Enable)
	} else if params.ESConfig.Enable {
		params.MessagingClient = messaging.NewKafkaClient(&s.cfg.Kafka, params.MetricsClient, zap.NewNop(), params.Logger, params.MetricScope, false, params.ESConfig.Enable)
	} else {
		params.MessagingClient = nil
	}

	// enable visibility to kafka and enable visibility to elastic search are using one config
	if params.ESConfig.Enable {
		esClient, err := elasticsearch.NewClient(&s.cfg.ElasticSearch)
		if err != nil {
			log.Fatalf("error creating elastic search client: %v", err)
		}
		params.ESClient = esClient

		indexName, ok := params.ESConfig.Indices[common.VisibilityAppName]
		if !ok || len(indexName) == 0 {
			log.Fatalf("elastic search config missing visibility index")
		}
	}

	dispatcher, err := params.DispatcherProvider.Get(common.FrontendServiceName, s.cfg.PublicClient.HostPort)
	if err != nil {
		log.Fatalf("failed to construct dispatcher: %v", err)
	}
	params.PublicClient = workflowserviceclient.New(dispatcher.ClientConfig(common.FrontendServiceName))

	if params.ClusterMetadata.ArchivalConfig().ConfiguredForArchival() {
		if s.cfg.Archival.Filestore != nil && s.cfg.Archival.S3store != nil {
			log.Fatalf("cannot config both filestore and s3store")
		}
		if s.cfg.Archival.Filestore == nil && s.cfg.Archival.S3store == nil {
			log.Fatalf("cannot config archival without filestore or s3store")
		}
		if s.cfg.Archival.Filestore != nil {
			filestoreClient, err := filestore.NewClient(s.cfg.Archival.Filestore)
			if err != nil {
				log.Fatalf("error creating file based blobstore: %v", err)
			}
			params.BlobstoreClient = filestoreClient
		}
		if s.cfg.Archival.S3store != nil {
			s3cli, err := s3store.ClientFromConfig(s.cfg.Archival.S3store)
			if err != nil {
				log.Fatalf("error creating s3 blobstore: %v", err)
			}
			params.BlobstoreClient = s3store.NewClient(s3cli)
		}
	}

	params.PersistenceConfig.TransactionSizeLimit = dc.GetIntProperty(dynamicconfig.TransactionSizeLimit, common.DefaultTransactionSizeLimit)

	params.Logger.Info("Starting service " + s.name)

	var daemon common.Daemon

	switch s.name {
	case frontendService:
		daemon = frontend.NewService(&params)
	case historyService:
		daemon = history.NewService(&params)
	case matchingService:
		daemon = matching.NewService(&params)
	case workerService:
		daemon = worker.NewService(&params)
	}

	go execute(daemon, s.doneC)

	return daemon
}

// execute runs the daemon in a separate go routine
func execute(d common.Daemon, doneC chan struct{}) {
	d.Start()
	close(doneC)
}
