//go:generate protoc --go_out=paths=source_relative:. --go-grpc_out=paths=source_relative:. score.proto
package proto