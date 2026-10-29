.PHONY: bootstrap test test-offline race vet dev build audit visual-audit visual-native feature-test release-gate
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
visual-audit:
	./tools/check-visual-style.py
	./tools/check-original-icons.py
	./tools/check-style-contracts.py
	./tools/check-native-structure.py
	./tools/trace-original-style.py
	./tools/check-visual-trace.py
visual-native:
	./tools/check-style-contracts.py --require-all
	./tools/check-visual-trace.py --release
	./tools/check-native-screenshots.sh

feature-test:
	./tools/check-feature-tests.sh domain
release-gate:
	./tools/check-feature-tests.sh release
	./tools/check-style-contracts.py --require-all
	./tools/check-visual-trace.py --release
	./tools/check-native-screenshots.sh
