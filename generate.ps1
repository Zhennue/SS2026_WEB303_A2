$ErrorActionPreference = 'Stop'
# Run from the project root after installing protoc and the Go plugins.
$env:Path += ";$(go env GOPATH)\bin"
protoc -I . --go_out=. --go_opt=module=novacart --go-grpc_out=. --go-grpc_opt=module=novacart proto/product.proto proto/order.proto
if ($LASTEXITCODE -ne 0) { throw 'Protocol Buffer code generation failed.' }
Write-Host 'Generated Go gRPC files in proto/.'
