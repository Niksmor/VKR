package main

import (
	"context"
	"embed"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"git.intermb.ru/im/parking/imparkingplatform/api_gateway/b2b-gateway.git/config"
	"git.intermb.ru/im/parking/imparkingplatform/observability/traces/tracer.git"

	handler_parkings_v1 "git.intermb.ru/im/parking/imparkingplatform/api_gateway/b2b-gateway.git/internal/handler/parkings/v1"
	handler_payments_v1 "git.intermb.ru/im/parking/imparkingplatform/api_gateway/b2b-gateway.git/internal/handler/payments/v1"
	handler_roadside_v1 "git.intermb.ru/im/parking/imparkingplatform/api_gateway/b2b-gateway.git/internal/handler/roadside/v1"
	handler_roadside_vrp_v1 "git.intermb.ru/im/parking/imparkingplatform/api_gateway/b2b-gateway.git/internal/handler/roadside_vrp/v1"
	handler_sms_sessions_v1 "git.intermb.ru/im/parking/imparkingplatform/api_gateway/b2b-gateway.git/internal/handler/sms_sessions/v1"
	handler_user_v1 "git.intermb.ru/im/parking/imparkingplatform/api_gateway/b2b-gateway.git/internal/handler/user/v1"
	"git.intermb.ru/im/parking/imparkingplatform/api_gateway/b2b-gateway.git/internal/integration"
	"git.intermb.ru/im/parking/imparkingplatform/api_gateway/b2b-gateway.git/internal/observ"
	server_v1 "git.intermb.ru/im/parking/imparkingplatform/api_gateway/b2b-gateway.git/internal/server/v1"
	services "git.intermb.ru/im/parking/imparkingplatform/api_gateway/b2b-gateway.git/internal/services"
	v1 "git.intermb.ru/im/parking/imparkingplatform/api_gateway/b2b-gateway.git/service/v1"
	"git.intermb.ru/im/parking/imparkingplatform/observability/logs/logger.git"
	log_consts "git.intermb.ru/im/parking/imparkingplatform/observability/logs/logger.git/consts"
	log_interceptors "git.intermb.ru/im/parking/imparkingplatform/observability/logs/logger.git/interceptors"
	"git.intermb.ru/im/parking/imparkingplatform/observability/traces/tracer.git"
	trace_interceptors "git.intermb.ru/im/parking/imparkingplatform/observability/traces/tracer.git/interceptors"
	trace_middlewares "git.intermb.ru/im/parking/imparkingplatform/observability/traces/tracer.git/middlewares"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/rs/cors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var (
	//go:embed api
	res embed.FS
)

// TODO: move it out of here (UnaryApiKeyServerInterceptor)

func main() {
	conn, err := grpc.Dial("tariffs-service:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to tariffs service: %v", err)
	}
	defer conn.Close()

	tariffClient := pb.NewTariffServiceClient(conn)
	parkingServer := handler.NewParkingServer(tariffClient)

	grpcServer := grpc.NewServer()
	pb.RegisterParkingServiceServer(grpcServer, parkingServer)

	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	log.Printf("Starting Parking gRPC server on port 50052")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
	cfg, err := config.NewConfig()
	if err != nil {
		panic(err)
}

	}

	log := logger.NewEntry(
		&logger.EntryConfig{
			ServiceName: "b2b-gateway",
			LogLevel:    cfg.LogLvl,
			LogFormat:   cfg.LogFormat,
		},
		nil,
		tracer.ExtractTraceSpanIds,
	)
	observ.SetLogger(log)
	loge := log.Entry()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	grpcAddr := ":" + strconv.Itoa(cfg.GRPCPort)
	restAddr := ":" + strconv.Itoa(cfg.RESTPort)

	// serve http
	var httpServer *http.Server
	go func() {

		mux := runtime.NewServeMux(
			runtime.WithMarshalerOption(
				runtime.MIMEWildcard,
				&runtime.JSONPb{
					MarshalOptions: protojson.MarshalOptions{
						EmitUnpopulated: true,
					},
				},
			),
			runtime.WithForwardResponseOption(func(ctx context.Context, resWriter http.ResponseWriter, pm proto.Message) error {
				trace_id, _ := tracer.ExtractTraceSpanIds(ctx)
				resWriter.Header().Add(log_consts.TraceIDField, trace_id)
				return nil
			}),
		)

		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithUnaryInterceptor(trace_interceptors.UnaryClientInterceptor(observ.Tracer, nil)),
		}
		err := v1.RegisterB2BGatewayHandlerFromEndpoint(ctx, mux, grpcAddr, opts)
		if err != nil {
			loge.WithError(err).Error("RegisterB2BGatewayHandlerFromEndpoint")
			return
		}

		mux.HandlePath("GET", "/swagger",
			func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
				file, _ := res.ReadFile("api/b2b_gateway_v1.swagger.json")
				w.Write(file)
			},
		)

		// cors.Default() setup the middleware with default options being
		// all origins accepted with simple methods (GET, POST). See
		// documentation below for more options.
		c := cors.New(cors.Options{
			AllowedOrigins:   []string{"http://localhost:3000", "*"},
			AllowCredentials: true,
			AllowedMethods: []string{
				"OPTIONS",
				"GET",
				"POST",
				"DELETE",
				"PUT",
				"PATCH",
			},
			AllowedHeaders: []string{"*"},
			// Enable Debugging for testing, consider disabling in production
			Debug: true, // TODO: fix this
		})

		loge.Infof("http listening on %s", restAddr)

		// TODO: timeout settings
		httpServer = &http.Server{
			Addr:    restAddr,
			Handler: trace_middlewares.HttpTraceMiddleware(observ.Tracer, c.Handler(mux)),
		}

		if err := httpServer.ListenAndServe(); err != nil {
			if err == http.ErrServerClosed {
				loge.Infof("http server stopped")
			} else {
				loge.WithError(err).Error("http server run")
			}
		}
	}()

	// serve grpc
	var grpcServer *grpc.Server
	go func() {
		lis, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			loge.WithError(err).Error("grpc listener")
			return
		}

		srv := server_v1.Server{}

		// Profile
		vehiclesApi, err := integration.NewVehiclesProvider(cfg.VehiclesServiceUrl)

		if err != nil {
			loge.WithError(err).Error("vehicles_service connect")
		}

		// TODO: check error scenario
		geoApi, err := integration.NewGeo(cfg.GeoServiceUrl)
		if err != nil {
			loge.WithError(err).Error("geo_service connect")
		}

		srv.SetParkingsHandler(handler_parkings_v1.NewHandler(geoApi, log))

		tariffsApi, err := integration.NewTariffs(cfg.TariffsServiceUrl)
		if err != nil {
			loge.WithError(err).Error("tariffs_service connect")
		}

		roadsideVrpApi, err := integration.NewRoadsideVrp(cfg.RoadsideVrpServiceUrl)
		if err != nil {
			loge.WithError(err).Error("roadside_vrp_service connect")
		}

		srv.SetRoadsideVrpHandler(handler_roadside_vrp_v1.NewHandler(tariffsApi, geoApi, roadsideVrpApi, log))

		authApi, err := integration.NewAuthProvider(cfg.AuthServiceUrl)

		if err != nil {
			loge.WithError(err).Error("auth_service connect")
		}

		srv.SetUserHandler(handler_user_v1.NewHandler(authApi, log))

		// Payments
		paymentsApi, err := integration.NewPayments(cfg.PaymentsServiceUrl)

		if err != nil {
			loge.WithError(err).Error("payments_service connect")
		}

		srv.SetPaymentsHandler(handler_payments_v1.NewHandler(paymentsApi, authApi))
		// Abonements
		abonementsApi, err := integration.NewAbonemetns(cfg.AbonementsServiceUrl)

		if err != nil {
			loge.WithError(err).Error("abonements service connect")
		}

		// Benefits
		benefitsApi, err := integration.NewBenefits(cfg.BenefitsServiceUrl)

		if err != nil {
			loge.WithError(err).Error("benefits service connect")
		}

		// Permissions
		permissionsApi, err := integration.NewPermissions(cfg.PermissionsServiceUrl)

		if err != nil {
			loge.WithError(err).Error("permissions service connect")
		}

		// Roadside
		roadsideApi, err := integration.NewRoadside(cfg.RoadsideServiceUrl)

		if err != nil {
			loge.WithError(err).Error("roadside_service connect")
		}

		srv.SetRoadsideSessionsHandler(handler_roadside_v1.NewHandler(
			roadsideApi,
			roadsideVrpApi,
			vehiclesApi,
			geoApi,
			abonementsApi,
			benefitsApi,
			permissionsApi,
		))

		// SmsSessions
		smsSessionsApi, err := integration.NewSmsSessions(cfg.SmsSessionsServiceUrl)

		if err != nil {
			loge.WithError(err).Error("sms_sessions_service connect")
		}

		srv.SetSmsSessionsHandler(handler_sms_sessions_v1.NewHandler(smsSessionsApi))

		logPayloadSettings := &log_interceptors.PayloadLogSettings{
			InboundEnabled: true,
			ExludeMethods: []string{
				"/grpc.health.v1.Health/Check",
				"/B2BGateway/Login",
				"/B2BGateway/RefreshToken",
			},
		}

		grpcServer = grpc.NewServer(
			grpc.UnaryInterceptor(
				grpc_middleware.ChainUnaryServer(
					trace_interceptors.UnaryServerInterceptor(observ.Tracer, nil),
					log_interceptors.UnaryServerInterceptor(loge, []string{
						"/grpc.health.v1.Health/Check",
					}, nil),
					services.UnaryApiKeyServerInterceptor(
						cfg,
					),
					services.UnaryValidateServerInterceptor(),
					log_interceptors.PayloadUnaryServerInterceptor(loge, logPayloadSettings),
				),
			),
		)

		grpc_health_v1.RegisterHealthServer(grpcServer, health.NewServer())

		v1.RegisterB2BGatewayServer(grpcServer, &srv)
		reflection.Register(grpcServer)

		loge.Infof("grpc listening on %s", grpcAddr)
		if err := grpcServer.Serve(lis); err != nil {
			loge.WithError(err).Error("grpc server run")
		}
	}()

	// shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	loge.Infof("service stop by signal: %s", sig.String())
	c, _ := context.WithTimeout(context.Background(), time.Second*time.Duration(cfg.ShutdownTimeout))
	go func() {
		<-c.Done()
		loge.Error("shutdown timeout")
		os.Exit(1)
	}()
	cancel()

	if err := httpServer.Shutdown(c); err != nil {
		loge.WithError(err).Error("http server shutdown")
	}

	grpcServer.GracefulStop()

	loge.Info("service stopped")
