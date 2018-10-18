.PHONY: run-test-grpc-server
run-test-grpc-server:
	#$(eval LIS_ADDR = $(shell go run tests/cmd/listener/main.go))
	#go run ./tests/cmd/test-server/main.go -a=$(LIS_ADDR)
	go run ./tests/cmd/test-server/main.go -a=127.0.0.1:60600 -t=120

.PHONY: run-test-grpc-proxy
run-test-grpc-proxy:
	go run ./tests/cmd/test-proxy/main.go -a=127.0.0.1:60601 -s=127.0.0.1:60600 -t=120

.PHONY: run-test-standalone
run-test-standalone:
	go test -v -coverprofile=proxy_coverage.out ./proxy/ -args -p=127.0.0.1:60601

.PHONY: run-test
run-test:
	go test -v ./proxy/ -args -a=$(LIS_ADDR)

.PHONY: test
test:
	$(eval LIS_ADDR_1 = $(shell go run tests/cmd/listener/main.go))
	$(eval LIS_ADDR_2 = $(shell go run tests/cmd/listener/main.go))
	go run ./tests/cmd/test-server/main.go -a=$(LIS_ADDR_1) -t=10 & \
		go run ./tests/cmd/test-proxy/main.go -a=$(LIS_ADDR_2) -s=$(LIS_ADDR_1) -t=10 & \
		sleep 2 && \
		go test -v -coverprofile=proxy_coverage.out ./proxy/ -args -p=$(LIS_ADDR_2)

.PHONY: cover-report
cover-report:
	go tool cover -html=proxy_coverage.out

