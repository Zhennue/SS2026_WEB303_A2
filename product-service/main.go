package main

import (
	"context"
	"database/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"log"
	"net"
	pb "novacart/proto"
	"os"
	"time"
)

type server struct {
	pb.UnimplementedProductServiceServer
	db *sql.DB
}

func (s *server) CreateProduct(ctx context.Context, p *pb.Product) (*pb.Product, error) {
	if p.GetId() == "" || p.GetName() == "" || p.GetPrice() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "id, name and a positive price are required")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO products(id,name,price,description) VALUES($1,$2,$3,$4)`, p.Id, p.Name, p.Price, p.Description)
	if err != nil {
		return nil, status.Error(codes.AlreadyExists, "product ID already exists or could not be saved")
	}
	return p, nil
}
func (s *server) GetProduct(ctx context.Context, id *pb.ProductID) (*pb.Product, error) {
	p := &pb.Product{}
	err := s.db.QueryRowContext(ctx, `SELECT id,name,price,description FROM products WHERE id=$1`, id.GetId()).Scan(&p.Id, &p.Name, &p.Price, &p.Description)
	if err == sql.ErrNoRows {
		return nil, status.Error(codes.NotFound, "product not found")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	return p, nil
}
func (s *server) ListProducts(ctx context.Context, _ *pb.Empty) (*pb.ProductList, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,price,description FROM products ORDER BY id`)
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	defer rows.Close()
	out := &pb.ProductList{}
	for rows.Next() {
		p := &pb.Product{}
		if err := rows.Scan(&p.Id, &p.Name, &p.Price, &p.Description); err != nil {
			return nil, status.Error(codes.Internal, "database error")
		}
		out.Products = append(out.Products, p)
	}
	return out, nil
}
func (s *server) UpdateProduct(ctx context.Context, p *pb.Product) (*pb.Product, error) {
	if p.GetId() == "" || p.GetName() == "" || p.GetPrice() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "id, name and a positive price are required")
	}
	r, err := s.db.ExecContext(ctx, `UPDATE products SET name=$2,price=$3,description=$4 WHERE id=$1`, p.Id, p.Name, p.Price, p.Description)
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return nil, status.Error(codes.NotFound, "product not found")
	}
	return p, nil
}
func (s *server) DeleteProduct(ctx context.Context, id *pb.ProductID) (*pb.DeleteResponse, error) {
	r, err := s.db.ExecContext(ctx, `DELETE FROM products WHERE id=$1`, id.GetId())
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return nil, status.Error(codes.NotFound, "product not found")
	}
	return &pb.DeleteResponse{Deleted: true}, nil
}
func main() {
	dsn := os.Getenv("PRODUCT_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://nova:nova_password@localhost:5433/products?sslmode=disable"
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
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS products(id TEXT PRIMARY KEY,name TEXT NOT NULL,price DOUBLE PRECISION NOT NULL CHECK(price>0),description TEXT NOT NULL DEFAULT '')`)
	if err != nil {
		log.Fatal(err)
	}
	l, err := net.Listen("tcp", ":"+env("PRODUCT_GRPC_PORT", "50051"))
	if err != nil {
		log.Fatal(err)
	}
	g := grpc.NewServer()
	pb.RegisterProductServiceServer(g, &server{db: db})
	reflection.Register(g)
	log.Println("Product Service listening on", l.Addr())
	log.Fatal(g.Serve(l))
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
