# NovaCart - WEB303 Assignment 1

Two collaborating Go microservices: Product Service (Service B) and Order Service (Service A), exposed through a REST API Gateway. Each service owns a separate PostgreSQL database. Internal service communication uses gRPC/Protocol Buffers.

## Requirements

- Go 1.23+
- Protocol Buffers compiler (`protoc`)
- Go plugins `protoc-gen-go` and `protoc-gen-go-grpc`
- Docker Desktop with Docker Compose

## Generate gRPC code (run once after editing .proto)

```powershell
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.5
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
```

Add `$(go env GOPATH)/bin` to PATH, then from project root:

```powershell
protoc --go_out=. --go_opt=module=novacart --go-grpc_out=. --go-grpc_opt=module=novacart proto/product.proto proto/order.proto
```

## Start everything

```powershell
go mod tidy
docker compose up --build
```

Gateway: http://localhost:8080/health

## Example API calls (PowerShell)

Create a product:

```powershell
Invoke-RestMethod -Method Post -Uri http://localhost:8080/products -ContentType 'application/json' -Body '{"id":"P001","name":"Laptop","price":75000,"description":"Student laptop"}'
```

List products:

```powershell
Invoke-RestMethod http://localhost:8080/products
```

Create an order (product must exist first):

```powershell
Invoke-RestMethod -Method Post -Uri http://localhost:8080/orders -ContentType 'application/json' -Body '{"id":"O001","product_id":"P001","quantity":2,"status":"CREATED"}'
```

List orders:

```powershell
Invoke-RestMethod http://localhost:8080/orders
```

Get one record: `GET /products/P001` or `GET /orders/O001`. Update with `PUT /products/P001` or `PUT /orders/O001`; delete with `DELETE /products/P001` or `DELETE /orders/O001`.

## Stop

```powershell
docker compose down
```

To also delete the local coursework databases: `docker compose down -v` (this permanently removes the stored test data).

## Architecture

Postman/REST client -> API Gateway -> Product/Order gRPC APIs. Order Service validates product existence by calling Product Service over gRPC. Product Service writes only to `product-db`; Order Service writes only to `order-db`. The Order-to-Product call uses a deadline, bounded retries with backoff, and a circuit breaker.

## Protoc .env setup

```
$env:Path = "E:\novacart\protoc-36.2-win64\bin;$env:Path"
```
