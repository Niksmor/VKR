module git.intermb.ru/im/parking/imparkingplatform/api_gateway/b2b-gateway.git

go 1.20

require (
	git.intermb.ru/im/parking/imparkingplatform/auth_services/auth_service_provider.git v0.4.4
	git.intermb.ru/im/parking/imparkingplatform/main_services/abonements.git v0.0.4
	git.intermb.ru/im/parking/imparkingplatform/main_services/benefits.git v0.0.6
	git.intermb.ru/im/parking/imparkingplatform/main_services/geo.git v0.2.0
	git.intermb.ru/im/parking/imparkingplatform/main_services/parking_sessions/roadside.git v0.1.0
	git.intermb.ru/im/parking/imparkingplatform/main_services/parking_sessions/roadside_vrp.git v0.0.7
	git.intermb.ru/im/parking/imparkingplatform/main_services/payments.git v0.1.0
	git.intermb.ru/im/parking/imparkingplatform/main_services/permissions.git v0.0.1
	git.intermb.ru/im/parking/imparkingplatform/main_services/tariffs.git v0.0.6
	git.intermb.ru/im/parking/imparkingplatform/main_services/vehicles.git v0.0.6
	git.intermb.ru/im/parking/imparkingplatform/observability/logs/logger.git v0.4.7
	git.intermb.ru/im/parking/imparkingplatform/observability/traces/tracer.git v0.1.5
	git.intermb.ru/im/parking/imparkingplatform/services/sms-sessions.git v0.0.1
	github.com/envoyproxy/protoc-gen-validate v1.0.2
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.18.1
	github.com/rs/cors v1.9.0
	github.com/sethvargo/go-envconfig v0.9.0
	go.opentelemetry.io/otel/trace v1.20.0
	google.golang.org/genproto/googleapis/api v0.0.0-20231106174013-bbf56f31fb17
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231106174013-bbf56f31fb17
	google.golang.org/grpc v1.59.0
	google.golang.org/protobuf v1.31.0

)

require (
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.44.0 // indirect
	google.golang.org/genproto v0.0.0-20231030173426-d783a09b4405 // indirect
)

require (
	git.intermb.ru/im/parking/imparkingplatform/observability/errors.git v0.0.2
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	go.opentelemetry.io/otel v1.20.0 // indirect
	go.opentelemetry.io/otel/exporters/jaeger v1.17.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.20.0 // indirect
	go.opentelemetry.io/otel/metric v1.20.0 // indirect
	go.opentelemetry.io/otel/sdk v1.20.0 // indirect
	golang.org/x/net v0.29.0 // indirect
	golang.org/x/sys v0.25.0 // indirect
	golang.org/x/text v0.18.0 // indirect
)
