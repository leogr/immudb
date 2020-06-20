/*
Copyright 2019-2020 vChain, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gw

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/codenotary/immudb/pkg/api/schema"
	immuclient "github.com/codenotary/immudb/pkg/client"
	"github.com/codenotary/immudb/pkg/client/auditor"
	"github.com/codenotary/immudb/pkg/client/cache"
	gwhandlers "github.com/codenotary/immudb/pkg/gw/handlers"
	"github.com/codenotary/immudb/pkg/server"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/rs/cors"
)

var startedAt time.Time

// Start starts the immudb gateway server
func (s *ImmuGwServer) Start() error {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cliOpts := &immuclient.Options{
		Dir:                s.Options.Dir,
		Address:            s.Options.ImmudbAddress,
		Port:               s.Options.ImmudbPort,
		HealthCheckRetries: 1,
		MTLs:               s.Options.MTLs,
		MTLsOptions:        s.Options.MTLsOptions,
		Auth:               true,
		Config:             "",
	}

	ic, err := immuclient.NewImmuClient(cliOpts)
	if err != nil {
		s.Logger.Errorf("unable to instantiate client: %s", err)
		return err
	}
	mux := runtime.NewServeMux()

	handler := cors.Default().Handler(mux)

	gbh := gwhandlers.NewGetBatchHandler(mux, ic)
	sbh := gwhandlers.NewSetBatchHandler(mux, ic)
	ish := gwhandlers.NewIScanHandler(mux, ic)
	ch := gwhandlers.NewCountHandler(mux, ic)
	bih := gwhandlers.NewByIndexHandler(mux, ic)
	gh := gwhandlers.NewGetHandler(mux, ic)
	sh := gwhandlers.NewSetHandler(mux, ic)
	ssh := gwhandlers.NewSafesetHandler(mux, ic)
	sgh := gwhandlers.NewSafegetHandler(mux, ic)
	hh := gwhandlers.NewHistoryHandler(mux, ic)
	sr := gwhandlers.NewSafeReferenceHandler(mux, ic)
	sza := gwhandlers.NewSafeZAddHandler(mux, ic)

	mux.Handle(http.MethodPost, schema.PatternImmuServiceGetBatch0(), gbh.GetBatch)
	mux.Handle(http.MethodPost, schema.PatternImmuServiceSetBatch0(), sbh.SetBatch)
	mux.Handle(http.MethodPost, schema.PatternImmuServiceIScan0(), ish.IScan)
	mux.Handle(http.MethodGet, schema.PatternImmuServiceCount0(), ch.Count)
	mux.Handle(http.MethodGet, schema.PatternImmuServiceByIndex0(), bih.ByIndex)
	mux.Handle(http.MethodGet, schema.PatternImmuServiceGet0(), gh.Get)
	mux.Handle(http.MethodPost, schema.PatternImmuServiceSet0(), sh.Set)
	mux.Handle(http.MethodPost, schema.PatternImmuServiceSafeSet0(), ssh.Safeset)
	mux.Handle(http.MethodPost, schema.PatternImmuServiceSafeGet0(), sgh.Safeget)
	mux.Handle(http.MethodGet, schema.PatternImmuServiceHistory0(), hh.History)
	mux.Handle(http.MethodPost, schema.PatternImmuServiceSafeReference0(), sr.SafeReference)
	mux.Handle(http.MethodPost, schema.PatternImmuServiceSafeZAdd0(), sza.SafeZAdd)

	err = schema.RegisterImmuServiceHandlerClient(ctx, mux, *ic.GetServiceClient())
	if err != nil {
		s.Logger.Errorf("unable to register client handlers: %s", err)
		return err
	}

	s.installShutdownHandler()
	s.Logger.Infof("starting immugw: %v", s.Options)
	if s.Options.Pidfile != "" {
		if s.Pid, err = server.NewPid(s.Options.Pidfile); err != nil {
			s.Logger.Errorf("failed to write pidfile: %s", err)
			return err
		}
	}

	if s.Options.Audit {
		defaultAuditor, err := auditor.DefaultAuditor(
			s.Options.AuditInterval,
			fmt.Sprintf("%s:%d", s.Options.ImmudbAddress, s.Options.ImmudbPort),
			cliOpts.DialOptions,
			s.Options.AuditUsername,
			s.Options.AuditPassword,
			cache.NewHistoryFileCache(filepath.Join(cliOpts.Dir, "auditor")),
			Metrics.UpdateAuditResult,
			nil)
		if err != nil {
			s.Logger.Errorf("unable to create auditor: %s", err)
			return err
		}
		auditorDone := make(chan struct{})
		go defaultAuditor.Run(s.Options.AuditInterval, ctx.Done(), auditorDone)
		defer func() { <-auditorDone }()
	}

	go func() {
		if err = http.ListenAndServe(s.Options.Address+":"+strconv.Itoa(s.Options.Port), handler); err != nil && err != http.ErrServerClosed {
			s.Logger.Errorf("unable to launch immugw: %+s", err)
		}
	}()

	metricsServer := StartMetrics(
		s.Options.MetricsBind(),
		s.Logger,
		func() float64 { return time.Since(startedAt).Hours() },
	)
	defer func() {
		if err = metricsServer.Close(); err != nil {
			s.Logger.Errorf("failed to shutdown metric server: %s", err)
		}
	}()
	startedAt = time.Now()
	<-s.quit
	if s.Options.Audit {
		cancel()
	}
	return err
}

// Stop stops the immudb gateway server
func (s *ImmuGwServer) Stop() error {
	s.Logger.Infof("stopping immugw: %v", s.Options)
	defer func() { s.quit <- struct{}{} }()
	return nil
}

func (s *ImmuGwServer) installShutdownHandler() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		defer func() {
			s.quit <- struct{}{}
		}()
		<-c
		s.Logger.Debugf("caught SIGTERM")
		if err := s.Stop(); err != nil {
			s.Logger.Errorf("shutdown error: %v", err)
		}
		s.Logger.Infof("shutdown completed")
	}()
}
