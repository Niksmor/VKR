package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"git.intermb.ru/im/parking/imparkingplatform/observability/license_helpers.git/check"
	license_interceptors "git.intermb.ru/im/parking/imparkingplatform/observability/license_helpers.git/interceptors"
	"git.intermb.ru/im/parking/imparkingplatform/observability/logs/logger.git"
	log_interceptors "git.intermb.ru/im/parking/imparkingplatform/observability/logs/logger.git/interceptors"
	"git.intermb.ru/im/parking/imparkingplatform/observability/traces/tracer.git"
	trace_interceptors "git.intermb.ru/im/parking/imparkingplatform/observability/traces/tracer.git/interceptors"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	rmqc "git.intermb.ru/im/parking/imparkingplatform/connectors/rabbitmq.git"
	pb "git.intermb.ru/im/parking/imparkingplatform/main_services/tariffs.git/api/pb"
	database "git.intermb.ru/im/parking/imparkingplatform/main_services/tariffs.git/db"
	"git.intermb.ru/im/parking/imparkingplatform/main_services/tariffs.git/internal/config"
	"git.intermb.ru/im/parking/imparkingplatform/main_services/tariffs.git/internal/events"
	sub_sessions_roadside "git.intermb.ru/im/parking/imparkingplatform/main_services/tariffs.git/internal/events/sessions_roadside"
	handler_calc_v1 "git.intermb.ru/im/parking/imparkingplatform/main_services/tariffs.git/internal/handler/calc/v1"
	handler_tariffs_v1 "git.intermb.ru/im/parking/imparkingplatform/main_services/tariffs.git/internal/handler/tariffs/v1"
	"git.intermb.ru/im/parking/imparkingplatform/main_services/tariffs.git/internal/integration"
	"git.intermb.ru/im/parking/imparkingplatform/main_services/tariffs.git/internal/observ"
	"git.intermb.ru/im/parking/imparkingplatform/main_services/tariffs.git/internal/repository"
	server_v1 "git.intermb.ru/im/parking/imparkingplatform/main_services/tariffs.git/internal/server/v1"
	services "git.intermb.ru/im/parking/imparkingplatform/main_services/tariffs.git/internal/services"
)

// TODO: check connections
// TODO: reconnect
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.NewConfig()

	if err != nil {
		panic(err)
	}

	log := logger.NewEntry(
		&logger.EntryConfig{
			ServiceName: cfg.AppName,
			LogLevel:    cfg.LogLvl,
			LogFormat:   cfg.LogFormat,
		},
		nil,
		tracer.ExtractTraceSpanIds,
	)
	observ.SetLogger(log)
	loge := log.Entry()
	// loge.Debugf("config: %+v", cfg)

	if tracer, err := tracer.NewTracer(ctx, &tracer.TracerConfig{
		ServiceName: cfg.AppName,
		Exporter: tracer.ExporterConfig{
			Type:        tracer.ExportTypeJaeger,
			Destination: cfg.TraceJaegerUrl,
		},
	}); err != nil {
		loge.WithError(err).Error("tracer initialization failed")
	} else {
		observ.SetTracer(tracer)
	}

	// Initialize Database
	db, dberr := database.Connect(
		fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable TimeZone=%s",
			cfg.DB.Host, cfg.DB.Port, cfg.DB.Username, cfg.DB.Password, cfg.DB.Database, "UTC"),
		cfg.LogQuery,
	)
	if dberr != nil {
		loge.WithError(dberr).Fatal("cannot connect to db")
	}

	loge.Debug("connected to database")

	// Initialize license check job
	err = check.NewLicenseCheckJob(
		ctx,
		loge,
		cfg.LicenseServiceUrl,
		30*time.Minute,
	)

	if err != nil {
		loge.Errorf("NewLicenseCheckJob: cannot get license restrictions err: %s", err.Error())
		return
	}

	trep := repository.NewTariffRepository(db)

	// rabbitmq
	// канал о закрытии коннекта к реббиту
	rmqCloseCh := rmqc.NewCloseCh(1)
	// подключение к реббиту
	rmqConn := rmqc.NewConnection(ctx, cfg.RabbitMQ.Url, rmqCloseCh)
	if err := rmqConn.Dial(); err != nil {
		loge.WithError(err).Fatal("cannot connect to rabbitmq")
	}

	// TODO: refactor, move to helper
	pubCalcStartRoadside, err := rmqc.NewPublisher(ctx, rmqConn, &rmqc.PublisherConfig{
		Name:  cfg.AppName,
		Route: cfg.RabbitMQ.Producers.Tariffs.RouteCalcStartRoadside,
		Exchange: rmqc.ExchangeConfig{
			Name: cfg.RabbitMQ.Producers.Tariffs.Exchange,
		},
		Declare: true,
	})
	if err != nil {
		loge.Fatalf(err.Error())
	}
	pubCalcStopRoadside, err := rmqc.NewPublisher(ctx, rmqConn, &rmqc.PublisherConfig{
		Name:  cfg.AppName,
		Route: cfg.RabbitMQ.Producers.Tariffs.RouteCalcStopRoadside,
		Exchange: rmqc.ExchangeConfig{
			Name: cfg.RabbitMQ.Producers.Tariffs.Exchange,
		},
		Declare: true,
	})
	if err != nil {
		loge.Fatalf(err.Error())
	}
	pubCalcFailRoadside, err := rmqc.NewPublisher(ctx, rmqConn, &rmqc.PublisherConfig{
		Name:  cfg.AppName,
		Route: cfg.RabbitMQ.Producers.Tariffs.RouteCalcFailRoadside,
		Exchange: rmqc.ExchangeConfig{
			Name: cfg.RabbitMQ.Producers.Tariffs.Exchange,
		},
		Declare: true,
	})
	if err != nil {
		loge.Fatalf(err.Error())
	}

	cRoadsideStart, err := rmqc.NewConsumer(ctx, rmqConn, &rmqc.ConsumerConfig{
		Name:         cfg.AppName,
		Queue:        "tariffs_sessions_roadside_started",
		BindingRoute: cfg.RabbitMQ.Consumers.SessionsRoadside.RouteStarted,
		Declare:      true,
		Retry: rmqc.RetryConfig{
			Enabled: true,
		},
		Exchange: rmqc.ExchangeConfig{
			Name: cfg.RabbitMQ.Consumers.SessionsRoadside.Exchange,
		},
	})
	if err != nil {
		loge.Fatalf(err.Error())
	}

	cRoadsideStop, err := rmqc.NewConsumer(ctx, rmqConn, &rmqc.ConsumerConfig{
		Name:         cfg.AppName,
		Queue:        "tariffs_sessions_roadside_stopped",
		BindingRoute: cfg.RabbitMQ.Consumers.SessionsRoadside.RouteStopped,
		Declare:      true,
		Retry: rmqc.RetryConfig{
			Enabled: true,
		},
		Exchange: rmqc.ExchangeConfig{
			Name: cfg.RabbitMQ.Consumers.SessionsRoadside.Exchange,
		},
	})
	if err != nil {
		loge.Fatalf(err.Error())
	}

	eventPublisher := events.NewEventPublisherRMQ()

	eventPublisher.SetPubCalcStartRoadside(pubCalcStartRoadside)
	eventPublisher.SetPubCalcStopRoadside(pubCalcStopRoadside)
	eventPublisher.SetPubCalcFailRoadside(pubCalcFailRoadside)

	hApi, err := integration.NewHolidaysProvider(cfg.HolidaysServiceUrl)
	if err != nil {
		loge.WithError(err).Error("holidays_service connect")
	}

	calcTimeLocation, err := time.LoadLocation(cfg.CalcTimezone)
	if err != nil {
		loge.Fatalf("load location for: %s err: %v", cfg.CalcTimezone, err)
	}

	roadsideSubHandler := sub_sessions_roadside.NewRoadsideHandler(trep, eventPublisher, hApi, calcTimeLocation) // TODO: move CalcTimezone to api parameters
	roadsideSub := sub_sessions_roadside.NewRoadsideSubscribeRMQ(roadsideSubHandler)
	roadsideSub.SetStartSession(cRoadsideStart)
	roadsideSub.SetStopSession(cRoadsideStop)
	go roadsideSub.Subscribe(ctx)

	// serve grpc
	var grpcServer *grpc.Server
	go func() {
		grpcAddr := ":" + strconv.Itoa(cfg.GRPCPort)

		lis, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			loge.WithError(err).Error("grpc listener")
			return
		}

		grpcServer = grpc.NewServer(
			grpc.UnaryInterceptor(
				grpc_middleware.ChainUnaryServer(
					trace_interceptors.UnaryServerInterceptor(observ.Tracer, nil),
					log_interceptors.UnaryServerInterceptor(observ.Log.Entry(), []string{
						"/grpc.health.v1.Health/Check",
					}, &log_interceptors.PayloadLogSettings{InboundEnabled: true, OutboundEnabled: true}),
					license_interceptors.LicenseUnaryServerInterceptor([]string{
						"/grpc.health.v1.Health/Check",
					}),
					services.UnaryValidateServerInterceptor(),
				),
			),
		)
		srv := &server_v1.Server{CalcTimeLocation: calcTimeLocation}

		// пробросим репозиторий в хэндлер, в котором находится бизнес логика
		lh := handler_tariffs_v1.NewTariffsHandler(trep)
		srv.SetTariffsHandler(lh)

		ch := handler_calc_v1.NewCalcHandler(trep, hApi)
		srv.SetCalcHandler(ch)

		grpc_health_v1.RegisterHealthServer(grpcServer, health.NewServer())

		pb.RegisterTariffsServer(grpcServer, srv)
		reflection.Register(grpcServer)

		loge.Infof("grpc listening on %s", grpcAddr)
		if err := grpcServer.Serve(lis); err != nil {
			loge.WithError(err).Error("grpc server run")
		}
	}()

	// shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

Loop:
	for {
		select {
		case sig := <-sigCh:
			loge.Warnf("service stop by signal: %s", sig.String())
			break Loop
		case err := <-rmqCloseCh:
			// todo: implement reconnect atepmts
			loge.WithError(err).Error("rmq connection")
			break Loop
		}
	}

	// todo: check is it required, ctx cancel will close connetction
	if err := rmqConn.Close(); err != nil {
		loge.Errorf("rmq connection close err: %s", err.Error())
	}
	c, _ := context.WithTimeout(context.Background(), time.Second*time.Duration(cfg.ShutdownTimeout))
	go func() {
		<-c.Done()
		loge.Error("shutdown timeout")
		os.Exit(1)
	}()
	cancel()

	grpcServer.GracefulStop()

	loge.Info("service stopped")
}
