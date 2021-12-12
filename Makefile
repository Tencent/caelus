REGISTRY ?= caelus

.PHONY: build
build: image
	image=${REGISTRY}/caelus:$$(cat VERSION); \
	mkdir -p _output/bin/; \
	docker run --rm $$image tar -cvf -  -C /binaries . | tar -xvf - -C _output/bin/

.PHONY: format
format:
	./hack/format.sh

.PHONY: test
test:
	./hack/test.sh

.PHONY: clean
clean:
	./hack/clean.sh

version:
	@version=$(VERSION); \
	[[ "$$version" != "" ]] || version="$$(git describe --dirty --always --tags | sed 's/-/./g')"; \
	touch VERSION && echo $$version > VERSION && echo image version is $$version

image: version
	image=${REGISTRY}/caelus:$$(cat VERSION); \
	echo building $$image;\
	docker build . -t $$image -f hack/Dockerfile
