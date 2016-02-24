//go:generate go-extpoints ./engines/extpoints/
//go:generate go-extpoints ./plugins/extpoints/
//go:generate go-import-subtree engines/ plugins/

package main

import (
	"fmt"
	"os"

	"github.com/docopt/docopt-go"
	"github.com/taskcluster/taskcluster-worker/config"
	"github.com/taskcluster/taskcluster-worker/engines/extpoints"
	"github.com/taskcluster/taskcluster-worker/runtime"
	"github.com/taskcluster/taskcluster-worker/worker"
)

const version = "taskcluster-worker 0.0.1"
const usage = `
TaskCluster worker
This worker is meant to be used with the taskcluster platform for the execution and
resolution of tasks.

  Usage:
    taskcluster-worker --help
    taskcluster-worker --version
    taskcluster-worker --engine <engine>
    taskcluster-worker --engine <engine> --logging-level <level>

  Options:
    --help  						Show this help screen.
    --version  						Display the version of go-import-subtree and exit.
    -e --engine <engine>  			Engine to use for task execution sandboxes.
    -l --logging-level <level>  	Set logging at <level>.
`

func main() {
	args, err := docopt.Parse(usage, nil, true, version, false, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing arguments. %v", err)
		os.Exit(1)
	}

	var level string
	if l := args["--logging-level"]; l != nil {
		level = l.(string)
	}
	logger, err := runtime.CreateLogger(level)
	if err != nil {
		os.Stderr.WriteString(err.Error())
		os.Exit(1)
	}

	e := args["--engine"]
	engineName := e.(string)

	engineProvider := extpoints.EngineProviders.Lookup(engineName)

	if engineProvider == nil {
		engineNames := extpoints.EngineProviders.Names()
		logger.Fatalf("Must supply a valid engine.  Supported Engines %v", engineNames)
	}

	runtimeEnvironment := runtime.Environment{Log: logger}

	engine, err := engineProvider.NewEngine(extpoints.EngineOptions{
		Environment: &runtimeEnvironment,
		Log:         logger.WithField("engine", engineName),
	})
	if err != nil {
		logger.Fatal(err.Error())
	}

	// TODO (garndt): Need to load up a real config in the future
	config := &config.Config{
		Credentials: struct {
			AccessToken string `json:"accessToken"`
			Certificate string `json:"certificate"`
			ClientId    string `json:"clientId"`
		}{
			AccessToken: "123",
			Certificate: "",
			ClientId:    "abc",
		},
		Capacity:      5,
		ProvisionerId: "tasckluster-worker-provisioner",
		WorkerGroup:   "taskcluster-worker-test-worker-group",
		WorkerId:      "taskcluster-worker-test-worker",
		QueueService: struct {
			ExpirationOffset int `json:"expirationOffset"`
		}{
			ExpirationOffset: 300,
		},
	}

	w := worker.New(config, &engine, &runtimeEnvironment, logger.WithField("component", "Task Manager"))

	runtimeEnvironment.Log.Debugf("Created worker %+v", w)
	runtimeEnvironment.Log.Info("Worker started up")
}
