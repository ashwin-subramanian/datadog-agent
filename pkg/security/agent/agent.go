package agent

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"google.golang.org/grpc"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RuntimeSecurityAgent represents the main wrapper for the Runtime Security product
type RuntimeSecurityAgent struct {
	logClient *DDClient
	conn      *grpc.ClientConn
	running   atomic.Value
	wg        *sync.WaitGroup
}

// NewRuntimeSecurityAgent instantiates a new RuntimeSecurityAgent
func NewRuntimeSecurityAgent() (*RuntimeSecurityAgent, error) {
	path := fmt.Sprintf("unix://%s", coreconfig.Datadog.GetString("runtime_security_config.socket"))
	conn, err := grpc.Dial(path, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return &RuntimeSecurityAgent{
		conn: conn,
		wg:   &sync.WaitGroup{},
		logClient: NewDDClientWithLogSource(config.NewLogSource(logSource, &config.LogsConfig{
			Type:    logType,
			Service: logService,
			Source:  logSource,
		})),
	}, nil
}

// Start the runtime security agent
func (rsa *RuntimeSecurityAgent) Start() {
	// Start the Datadog log client. This client is used to ship security events to Datadog.
	go rsa.logClient.Run(rsa.wg)
	// Start the system-probe events listener
	go rsa.StartEventListener()
}

// Stop the runtime recurity agent
func (rsa *RuntimeSecurityAgent) Stop() {
	rsa.running.Store(false)
	rsa.logClient.Stop()
	rsa.wg.Wait()
	rsa.conn.Close()
}

// StartEventListener starts listening for new events from system-probe
func (rsa *RuntimeSecurityAgent) StartEventListener() {
	rsa.wg.Add(1)
	defer rsa.wg.Done()
	apiClient := api.NewSecurityModuleClient(rsa.conn)

	rsa.running.Store(true)
	for rsa.running.Load() == true {
		stream, err := apiClient.GetEvents(context.Background(), &api.GetParams{})
		if err != nil {
			log.Warnf("grpc stream connection error: %v", err)
			// retry in 2 seconds
			time.Sleep(2 * time.Second)
			continue
		}

		for {
			// Get new event from stream
			in, err := stream.Recv()
			if err == io.EOF || in == nil {
				break
			}
			log.Infof("Got message from rule `%s` for event `%s` with tags `%+v` ", in.RuleID, string(in.Data), in.Tags)

			// Dispatch security event
			rsa.DispatchEvent(in)
		}
	}
}

// DispatchEvent dispatches a security event message to the subsytems of the runtime security agent
func (rsa *RuntimeSecurityAgent) DispatchEvent(evt *api.SecurityEventMessage) {
	// For now simply log to Datadog
	rsa.logClient.SendSecurityEvent(evt, message.StatusAlert)
}
