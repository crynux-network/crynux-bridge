package main

import (
	"context"
	"crynux_bridge/api"
	"crynux_bridge/config"
	"crynux_bridge/migrate"
	"crynux_bridge/relay"
	"crynux_bridge/taskengine"
	"crynux_bridge/tasks"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
)

func main() {
	if err := config.InitConfig(""); err != nil {
		print("Error reading config file")
		print(err.Error())
		os.Exit(1)
	}

	conf := config.GetConfig()

	if err := config.InitLog(conf); err != nil {
		print("Error initializing log")
		print(err.Error())
		os.Exit(1)
	}

	if err := config.InitDB(conf); err != nil {
		log.Fatalln(err.Error())
	}

	startDBMigration()

	// Check the relay account balance
	if err := relay.CheckBalanceForTaskCreator(context.Background()); err != nil {
		log.Fatalln(err)
	}
	if err := config.DeleteBlockchainPrivateKeyFileAfterRead(); err != nil {
		log.Fatalln(err)
	}

	engine, err := taskengine.New(config.GetDB(), taskengine.Config{
		DefaultRepeatNum:             conf.Task.RepeatNum,
		ScanInterval:                 conf.Task.Engine.ScanInterval,
		StatusPollInterval:           conf.Task.Engine.StatusPollInterval,
		ExecutionPollAdvance:         conf.Task.Engine.ExecutionPollAdvance,
		ExecutionOverrunPollInterval: conf.Task.Engine.ExecutionOverrunPollInterval,
		RetryInterval:                conf.Task.Engine.RetryInterval,
		OperationTimeout:             conf.Task.Engine.OperationTimeout,
		ExpansionBatchSize:           conf.Task.Engine.ExpansionBatchSize,
		OperationResultBatchSize:     conf.Task.Engine.OperationResultBatchSize,
		DueTaskBatchSize:             conf.Task.Engine.DueTaskBatchSize,
		CreateBatchSize:              conf.Task.Engine.CreateBatchSize,
		StatusBatchSize:              conf.Task.Engine.StatusBatchSize,
		ValidationBatchSize:          conf.Task.Engine.ValidationBatchSize,
		CancellationBatchSize:        conf.Task.Engine.CancellationBatchSize,
		CreateWorkers:                conf.Task.Engine.CreateWorkers,
		StatusWorkers:                conf.Task.Engine.StatusWorkers,
		ValidationWorkers:            conf.Task.Engine.ValidationWorkers,
		CancellationWorkers:          conf.Task.Engine.CancellationWorkers,
		ResultWorkers:                conf.Task.Engine.ResultWorkers,
		ResultDirectory:              conf.DataDir.InferenceTasks,
		PrivateKey:                   conf.Blockchain.Account.PrivateKey,
	})
	if err != nil {
		log.Fatalln(err)
	}
	taskengine.SetDefault(engine)
	engine.Start(context.Background())
	go tasks.ProcessTasks(context.Background())
	go tasks.HeartbeatCreateTasks(context.Background())
	go tasks.ProcessSDFTTasks(context.Background())
	go tasks.CleanupInferenceResults(context.Background())

	startServer()
}

func startServer() {
	conf := config.GetConfig()

	app := api.GetHttpApplication(conf)
	address := fmt.Sprintf("%s:%s", conf.Http.Host, conf.Http.Port)

	log.Infoln("Starting application server...")

	if err := app.Run(address); err != nil {
		log.Fatalln(err)
	}
}

func startDBMigration() {

	migrate.InitMigration(config.GetDB())

	if err := migrate.Migrate(); err != nil {
		log.Fatalln(err)
	}

	log.Infoln("DB migrations are done!")
}
