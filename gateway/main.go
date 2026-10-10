package main

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"log"
	"net/http"
	pb "novacart/proto"
	"os"
	"strings"
	"time"
)

type api struct {
	products pb.ProductServiceClient
	orders   pb.OrderServiceClient
}

func (a *api) respond(w http.ResponseWriter, value any, err error, success int) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		code := http.StatusInternalServerError
		switch status.Code(err) {
		case codes.InvalidArgument:
			code = http.StatusBadRequest
		case codes.NotFound:
			code = http.StatusNotFound
		case codes.AlreadyExists:
			code = http.StatusConflict
		case codes.FailedPrecondition:
			code = http.StatusUnprocessableEntity
		case codes.Unavailable:
			code = http.StatusServiceUnavailable
		case codes.DeadlineExceeded:
			code = http.StatusGatewayTimeout
		}
		if status.Code(err) == codes.Unknown {
			var ne interface{ Error() string }
			if errors.As(err, &ne) && strings.Contains(err.Error(), "unavailable") {
				code = http.StatusServiceUnavailable
			}
		}
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(success)
	_ = json.NewEncoder(w).Encode(value)
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return false
	}
	return true
}
func (a *api) productsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	id := r.PathValue("id")
	switch r.Method {
	case http.MethodPost:
		var p pb.Product
		if !decode(w, r, &p) {
			return
		}
		v, e := a.products.CreateProduct(ctx, &p)
		a.respond(w, v, e, http.StatusCreated)
	case http.MethodGet:
		if id != "" {
			v, e := a.products.GetProduct(ctx, &pb.ProductID{Id: id})
			a.respond(w, v, e, http.StatusOK)
		} else {
			v, e := a.products.ListProducts(ctx, &pb.Empty{})
			a.respond(w, v, e, http.StatusOK)
		}
	case http.MethodPut:
		var p pb.Product
		if !decode(w, r, &p) {
			return
		}
		if id != "" {
			p.Id = id
		}
		v, e := a.products.UpdateProduct(ctx, &p)
		a.respond(w, v, e, http.StatusOK)
	case http.MethodDelete:
		if id == "" {
			a.respond(w, nil, status.Error(codes.InvalidArgument, "product ID is required"), 0)
			return
		}
		v, e := a.products.DeleteProduct(ctx, &pb.ProductID{Id: id})
		a.respond(w, v, e, http.StatusOK)
	default:
		w.Header().Set("Allow", "GET, POST, PUT, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
func (a *api) ordersHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	id := r.PathValue("id")
	switch r.Method {
	case http.MethodPost:
		var o pb.Order
		if !decode(w, r, &o) {
			return
		}
		v, e := a.orders.CreateOrder(ctx, &o)
		a.respond(w, v, e, http.StatusCreated)
	case http.MethodGet:
		if id != "" {
			v, e := a.orders.GetOrder(ctx, &pb.OrderID{Id: id})
			a.respond(w, v, e, http.StatusOK)
		} else {
			v, e := a.orders.ListOrders(ctx, &pb.Empty{})
			a.respond(w, v, e, http.StatusOK)
		}
	case http.MethodPut:
		var o pb.Order
		if !decode(w, r, &o) {
			return
		}
		if id != "" {
			o.Id = id
		}
		v, e := a.orders.UpdateOrder(ctx, &o)
		a.respond(w, v, e, http.StatusOK)
	case http.MethodDelete:
		if id == "" {
			a.respond(w, nil, status.Error(codes.InvalidArgument, "order ID is required"), 0)
			return
		}
		v, e := a.orders.DeleteOrder(ctx, &pb.OrderID{Id: id})
		a.respond(w, v, e, http.StatusOK)
	default:
		w.Header().Set("Allow", "GET, POST, PUT, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
func main() {
	pc, err := grpc.NewClient(env("PRODUCT_GRPC_ADDR", "localhost:50051"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()
	oc, err := grpc.NewClient(env("ORDER_GRPC_ADDR", "localhost:50052"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer oc.Close()
	a := &api{products: pb.NewProductServiceClient(pc), orders: pb.NewOrderServiceClient(oc)}
	mux := http.NewServeMux()
	mux.HandleFunc("/products", a.productsHandler)
	mux.HandleFunc("/products/{id}", a.productsHandler)
	mux.HandleFunc("/orders", a.ordersHandler)
	mux.HandleFunc("/orders/{id}", a.ordersHandler)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	addr := ":" + env("GATEWAY_PORT", "8080")
	log.Println("API Gateway listening on", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
