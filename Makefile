all: clean build-docker-image test-docker-image

clean:
	rm -rf *.so *.h *~

build-plugin:
	go build -buildmode=c-shared -o out_dogstatsd_metrics.so

build-docker-image:
	docker build -t bin3377/fluent-bit-out-dogstatsd-metrics:latest -f Dockerfile .

test-docker-image:
	docker run --rm bin3377/fluent-bit-out-dogstatsd-metrics:latest
