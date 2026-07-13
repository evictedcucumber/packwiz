.PHONY: build
build:
	cargo build

.PHONY: test
test:
	cargo nextest run

.PHONY: coverage
coverage:
	cargo tarpaulin --fail-under 80

.PHONY: dependencies
dependencies:
	cargo audit
	cargo deny check

.PHONY: fmt
fmt:
	cargo fmt
