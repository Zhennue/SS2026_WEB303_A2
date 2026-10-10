# NovaCart - WEB303 Assignment 2

Two collaborating Go microservices behind a REST API Gateway, using gRPC and Protocol Buffers.

- **Product Service (Service B)** owns products and has its own PostgreSQL database (`products`).
- **Order Service (Service A)** owns orders and has its own PostgreSQL database (`orders`). It calls Product Service over gRPC to check that a product exists and to read its price.
- **API Gateway** is the only public entry point. It accepts REST/JSON and calls the services over gRPC.

## Architecture

Each service reads and writes only its own database. Order Service gets product data only through Product Service's gRPC API.

## Run it

You only need Docker Desktop (with Docker Compose).

```powershell
docker compose up --build
```

Check that it works: http://localhost:8080/health should return `{"status":"ok"}`.

Stop everything:

```powershell
docker compose down
```

Add `-v` to also delete the stored test data: `docker compose down -v`.

Containers started: `product-db`, `order-db`, `product-service`, `order-service`, `gateway`.

## Test with Postman

1. Open Postman and choose **Import**.
2. Select `postman/NovaCart.postman_collection.json`.
3. Run the requests from top to bottom (create product first, then create order).

## API (all calls go through the gateway on port 8080)

| Method | URL                              | Success code |
| ------ | -------------------------------- | ------------ |
| POST   | `/products`, `/orders`           | 201          |
| GET    | `/products`, `/orders`           | 200          |
| GET    | `/products/{id}`, `/orders/{id}` | 200          |
| PUT    | `/products/{id}`, `/orders/{id}` | 200          |
| DELETE | `/products/{id}`, `/orders/{id}` | 200          |
| GET    | `/health`                        | 200          |

Example bodies:

```json
{"id":"P001","name":"Laptop","price":75000,"description":"Student laptop"}
{"id":"O001","product_id":"P001","quantity":2,"status":"CREATED"}
```

An order's `total` is calculated by the Order Service from the product price (price x quantity).

## Error mapping (gRPC to HTTP)

| gRPC code          | HTTP status | Example                                        |
| ------------------ | ----------- | ---------------------------------------------- |
| InvalidArgument    | 400         | missing field, bad JSON                        |
| NotFound           | 404         | product or order id does not exist             |
| AlreadyExists      | 409         | id already used                                |
| FailedPrecondition | 422         | order for a product that does not exist        |
| Unavailable        | 503         | Product Service is down or the circuit is open |
| DeadlineExceeded   | 504         | call took too long                             |
| anything else      | 500         | unexpected error                               |

## Resilience (Order Service to Product Service)

All calls go through `resilience.Do()` in the `resilience/` folder:

- **Timeout:** 2 seconds per attempt.
- **Retry with backoff:** up to 3 attempts, waiting about 100 ms then 200 ms (plus a small random jitter).
- **Circuit breaker:** opens after 3 failed attempts and stays open for 5 seconds. While it is open, calls fail immediately with a clear 503 message. After 5 seconds it allows calls again.
  To try it:

1. `docker compose stop product-service`
2. Send `POST /orders` (you get 503 after the retries).
3. Send it again straight away (instant 503, "circuit breaker is open").
4. `docker compose start product-service`, wait 10 seconds, send it again (201).

## Validation

- **Product:** `id` and `name` are required, `price` must be greater than 0.
- **Order:** `id` and `product_id` are required, `quantity` must be greater than 0.
- **Gateway:** the body must be valid JSON with only known fields.
  Invalid requests return 400 and are not stored. The databases also have `CHECK` rules (`price > 0`, `quantity > 0`).

## Configuration

Ports, database URLs and service addresses are read from environment variables. They are set in the `environment:` sections of `docker-compose.yml`.

| Variable               | Used by                | Meaning                    |
| ---------------------- | ---------------------- | -------------------------- |
| `GATEWAY_PORT`         | gateway                | REST port (8080)           |
| `PRODUCT_GRPC_ADDR`    | gateway, order-service | address of Product Service |
| `ORDER_GRPC_ADDR`      | gateway                | address of Order Service   |
| `PRODUCT_GRPC_PORT`    | product-service        | gRPC port (50051)          |
| `PRODUCT_DATABASE_URL` | product-service        | connection to `product-db` |
| `ORDER_GRPC_PORT`      | order-service          | gRPC port (50052)          |
| `ORDER_DATABASE_URL`   | order-service          | connection to `order-db`   |

## Folder layout

```
gateway/           REST API gateway
product-service/   Service B
order-service/     Service A
resilience/        timeout, retry and circuit breaker helper
proto/             .proto files and generated Go code
postman/           Postman collection
Dockerfile.*       multi-stage Dockerfiles for each Go service
docker-compose.yml starts all 5 containers
```

## Troubleshooting

- **Gateway does not start, "port is not available" on 8080:** another program is using the port. Find it with `netstat -ano | findstr :8080`, close it, then run `docker compose up -d`.
- **Order requests fail right after a "product not found" error:** the circuit breaker may be open. Wait 5 to 10 seconds and try again.

## Only if you edit the .proto files

Install `protoc` and the Go plugins:

```powershell
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.5
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
```

Make sure `protoc` and `$(go env GOPATH)/bin` are on your PATH, then from the project root:

```powershell
protoc --go_out=. --go_opt=module=novacart --go-grpc_out=. --go-grpc_opt=module=novacart proto/product.proto proto/order.proto
```

Then run `go mod tidy` and `docker compose up --build`.
