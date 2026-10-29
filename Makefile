.PHONY: bootstrap test test-offline race vet dev build audit feature-test release-gate
bootstrap:
	./tools/bootstrap.sh
test:
	./tools/verify.sh
test-offline:
	./tools/test-offline.sh
race:
	./tools/test-offline.sh race
vet:
	./tools/test-offline.sh vet
dev:
	go tool mygo dev
build:
	./tools/build-macos.sh
audit:
	./tools/check-source.sh

feature-test:
	./tools/check-feature-tests.sh domain
release-gate:
	./tools/check-feature-tests.sh release
