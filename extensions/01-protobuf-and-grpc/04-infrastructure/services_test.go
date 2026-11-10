package main

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	reflectionv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
	reflectionalpha "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/status"
)

func TestHealthChecksAndProtectedWatch(t *testing.T) {
	env := setup(t, false, nil)
	client := healthv1.NewHealthClient(env.connection(t, ""))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for _, name := range []string{"", contactService} {
		response, err := client.Check(ctx, &healthv1.HealthCheckRequest{Service: name})
		if err != nil || response.GetStatus() != healthv1.HealthCheckResponse_SERVING {
			t.Fatalf("readiness for %q = %v, %v", name, response, err)
		}
	}
	if _, err := client.Check(ctx, &healthv1.HealthCheckRequest{Service: "unknown"}); status.Code(err) != codes.NotFound {
		t.Fatalf("unknown service = %v", err)
	}
	if _, err := client.List(ctx, &healthv1.HealthListRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unauthenticated health list = %v", err)
	}
	stream, err := client.Watch(ctx, &healthv1.HealthCheckRequest{Service: contactService})
	if err == nil {
		_, err = stream.Recv()
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unauthenticated watch = %v", err)
	}
	reader := healthv1.NewHealthClient(env.connection(t, env.secrets.reader))
	listed, err := reader.List(ctx, &healthv1.HealthListRequest{})
	if err != nil || listed.GetStatuses()[contactService].GetStatus() != healthv1.HealthCheckResponse_SERVING {
		t.Fatalf("reader health list = %v, %v", listed, err)
	}
}

func TestHealthWatchTransitionsAndShutdown(t *testing.T) {
	for _, force := range []bool{false, true} {
		name := "graceful after client cancellation"
		if force {
			name = "forced stop of an open watch"
		}
		t.Run(name, func(t *testing.T) {
			env := setup(t, false, nil)
			client := healthv1.NewHealthClient(env.connection(t, env.secrets.reader))
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			stream, err := client.Watch(ctx, &healthv1.HealthCheckRequest{Service: contactService})
			if err != nil {
				t.Fatal(err)
			}
			check := func(want healthv1.HealthCheckResponse_ServingStatus) {
				t.Helper()
				response, err := stream.Recv()
				if err != nil || response.GetStatus() != want {
					t.Fatalf("watch = %v, %v; want %s", response, err, want)
				}
			}
			check(healthv1.HealthCheckResponse_SERVING)
			env.health.SetServingStatus(contactService, healthv1.HealthCheckResponse_NOT_SERVING)
			check(healthv1.HealthCheckResponse_NOT_SERVING)
			env.health.SetServingStatus(contactService, healthv1.HealthCheckResponse_SERVING)
			check(healthv1.HealthCheckResponse_SERVING)
			shutdownContext, stop := context.WithCancel(context.Background())
			defer stop()
			done := make(chan error, 1)
			go func() { done <- env.server.Shutdown(shutdownContext) }()
			check(healthv1.HealthCheckResponse_NOT_SERVING)
			select {
			case err := <-done:
				t.Fatalf("shutdown did not wait for the open Watch: %v", err)
			default:
			}
			if force {
				stop() // Exhaust the drain budget while the Watch is still open.
			} else {
				cancel() // Cooperative clients release long-lived subscriptions.
			}
			_, terminal := stream.Recv()
			if terminal == nil || terminal == io.EOF {
				t.Fatalf("an interrupted Watch must have a failure status: %v", terminal)
			}
			select {
			case err := <-done:
				if force && !errors.Is(err, context.Canceled) || !force && err != nil {
					t.Fatalf("shutdown: %v, force=%t", err, force)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("shutdown did not join the health handler")
			}
			response, err := env.health.Check(t.Context(), &healthv1.HealthCheckRequest{Service: contactService})
			if err != nil || response.GetStatus() != healthv1.HealthCheckResponse_NOT_SERVING {
				t.Fatalf("final readiness = %v, %v", response, err)
			}
		})
	}
}

func listReflection(ctx context.Context, connection *grpc.ClientConn, alpha bool) error {
	if alpha {
		stream, err := reflectionalpha.NewServerReflectionClient(connection).ServerReflectionInfo(ctx)
		if err != nil {
			return err
		}
		request := &reflectionalpha.ServerReflectionRequest{MessageRequest: &reflectionalpha.ServerReflectionRequest_ListServices{ListServices: ""}}
		if err := stream.Send(request); err != nil && err != io.EOF {
			return err
		}
		response, err := stream.Recv()
		if err != nil {
			return err
		}
		if len(response.GetListServicesResponse().GetService()) == 0 {
			return errors.New("reflection returned no service descriptors")
		}
		if err := stream.CloseSend(); err != nil {
			return err
		}
		if _, err := stream.Recv(); err != io.EOF {
			return errors.New("reflection did not complete with EOF")
		}
		return nil
	}
	stream, err := reflectionv1.NewServerReflectionClient(connection).ServerReflectionInfo(ctx)
	if err != nil {
		return err
	}
	request := &reflectionv1.ServerReflectionRequest{MessageRequest: &reflectionv1.ServerReflectionRequest_ListServices{ListServices: ""}}
	if err := stream.Send(request); err != nil && err != io.EOF {
		return err
	}
	response, err := stream.Recv()
	if err != nil {
		return err
	}
	if len(response.GetListServicesResponse().GetService()) == 0 {
		return errors.New("reflection returned no service descriptors")
	}
	if err := stream.CloseSend(); err != nil {
		return err
	}
	if _, err := stream.Recv(); err != io.EOF {
		return errors.New("reflection did not complete with EOF")
	}
	return nil
}

func TestReflectionOptInAndAccessPolicy(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			env := setup(t, enabled, nil)
			for _, tc := range []struct {
				token string
				code  codes.Code
			}{{"", codes.Unauthenticated}, {env.secrets.reader, codes.PermissionDenied}, {env.secrets.writer, codes.OK}} {
				connection := env.connection(t, tc.token)
				for _, alpha := range []bool{false, true} {
					ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
					err := listReflection(ctx, connection, alpha)
					cancel()
					want := tc.code
					if !enabled {
						want = codes.Unimplemented
					}
					if status.Code(err) != want {
						t.Fatalf("reflection enabled=%t alpha=%t: %v, want %s", enabled, alpha, err, want)
					}
				}
			}
		})
	}
}
