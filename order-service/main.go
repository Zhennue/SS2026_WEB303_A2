package main

import (
	"context"
	"database/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"log"
	"net"
	pb "novacart/proto"
	"novacart/resilience"
	"os"
	"time"
)

type server struct {
	pb.UnimplementedOrderServiceServer
	db       *sql.DB
	products pb.ProductServiceClient
	breaker  *resilience.Breaker
}

func (s *server) CreateOrder(ctx context.Context, o *pb.Order) (*pb.Order, error) {
	if o.GetId() == "" || o.GetProductId() == "" || o.GetQuantity() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "id, product_id and positive quantity are required")
	}
	p, err := resilience.Do(ctx, 2*time.Second, 3, s.breaker, func(c context.Context) (*pb.Product, error) {
		return s.products.GetProduct(c, &pb.ProductID{Id: o.ProductId})
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, status.Error(codes.FailedPrecondition, "product does not exist")
		}
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	o.Total = p.Price * float64(o.Quantity)
	if o.Status == "" {
		o.Status = "CREATED"
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO orders(id,product_id,quantity,status,total) VALUES($1,$2,$3,$4,$5)`, o.Id, o.ProductId, o.Quantity, o.Status, o.Total)
	if err != nil {
		return nil, status.Error(codes.AlreadyExists, "order ID already exists or could not be saved")
	}
	return o, nil
}
func (s *server) GetOrder(ctx context.Context, id *pb.OrderID) (*pb.Order, error) {
	o := &pb.Order{}
	err := s.db.QueryRowContext(ctx, `SELECT id,product_id,quantity,status,total FROM orders WHERE id=$1`, id.GetId()).Scan(&o.Id, &o.ProductId, &o.Quantity, &o.Status, &o.Total)
	if err == sql.ErrNoRows {
		return nil, status.Error(codes.NotFound, "order not found")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	return o, nil
}
func (s *server) ListOrders(ctx context.Context, _ *pb.Empty) (*pb.OrderList, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,product_id,quantity,status,total FROM orders ORDER BY id`)
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	defer rows.Close()
	out := &pb.OrderList{}
	for rows.Next() {
		o := &pb.Order{}
		if err := rows.Scan(&o.Id, &o.ProductId, &o.Quantity, &o.Status, &o.Total); err != nil {
			return nil, status.Error(codes.Internal, "database error")
		}
		out.Orders = append(out.Orders, o)
	}
	return out, nil
}
func (s *server) UpdateOrder(ctx context.Context, o *pb.Order) (*pb.Order, error) {
	if o.GetId() == "" || o.GetProductId() == "" || o.GetQuantity() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "id, product_id and positive quantity are required")
	}
	p, err := resilience.Do(ctx, 2*time.Second, 3, s.breaker, func(c context.Context) (*pb.Product, error) {
		return s.products.GetProduct(c, &pb.ProductID{Id: o.ProductId})
	})
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, "cannot update order: product is unavailable or does not exist")
	}
	o.Total = p.Price * float64(o.Quantity)
	r, err := s.db.ExecContext(ctx, `UPDATE orders SET product_id=$2,quantity=$3,status=$4,total=$5 WHERE id=$1`, o.Id, o.ProductId, o.Quantity, o.Status, o.Total)
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return nil, status.Error(codes.NotFound, "order not found")
	}
	return o, nil
}
func (s *server) DeleteOrder(ctx context.Context, id *pb.OrderID) (*pb.DeleteResponse, error) {
	r, err := s.db.ExecContext(ctx, `DELETE FROM orders WHERE id=$1`, id.GetId())
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return nil, status.Error(codes.NotFound, "order not found")
	}
	return &pb.DeleteResponse{Deleted: true}, nil
}
func main() {
	dsn := os.Getenv("ORDER_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://nova:nova_password@localhost:5434/orders?sslmode=disable"
	}
	var db *sql.DB
	var err error
	for i := 0; i < 30; i++ {
		db, err = sql.Open("pgx", dsn)
		if err == nil {
			err = db.Ping()
		}
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS orders(id TEXT PRIMARY KEY,product_id TEXT NOT NULL,quantity INTEGER NOT NULL CHECK(quantity>0),status TEXT NOT NULL,total DOUBLE PRECISION NOT NULL)`)
	if err != nil {
		log.Fatal(err)
	}
	pc, err := grpc.NewClient(env("PRODUCT_GRPC_ADDR", "product-service:50051"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()
	l, err := net.Listen("tcp", ":"+env("ORDER_GRPC_PORT", "50052"))
	if err != nil {
		log.Fatal(err)
	}
	g := grpc.NewServer()
	pb.RegisterOrderServiceServer(g, &server{db: db, products: pb.NewProductServiceClient(pc), breaker: resilience.NewBreaker(3, 5*time.Second)})
	reflection.Register(g)
	log.Println("Order Service listening on", l.Addr())
	log.Fatal(g.Serve(l))
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
