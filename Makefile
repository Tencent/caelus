.PHONY: build
build: format
	./hack/build.sh

.PHONY: format
format:
	./hack/format.sh

.PHONY: test
test:
	./hack/test.sh

.PHONY: clean
clean:
	./hack/clean.sh

.PHONY: image
image: build
	./hack/image.sh
