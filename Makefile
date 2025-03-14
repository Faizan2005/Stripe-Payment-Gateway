build:
		@go build -o bin/payment

run: build
		@DB_HOST=localhost DB_PORT=5432 DB_USER=postgres DB_NAME=payment_gateway ./bin/payment 

test:
		@go test -v ./..