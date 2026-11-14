.PHONY: build test vet clean

build:
	go build -o epirun .

test:
	go test ./...

vet:
	go vet ./...
	gofmt -l . | (! grep .)

clean:
	rm -f epirun
