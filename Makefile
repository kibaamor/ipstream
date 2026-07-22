.PHONY: build test vet bench bench-parse-stats fuzz bce coverage profile-cpu profile-cpu-reports clean lint lint-fix check

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X main.version=$(VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.buildDate=$(BUILD_DATE)
STATS_BUILD_FLAGS ?= -tags='ipstreamstats'
BENCH_TIME ?= 3s
PARSE_STATS_BENCH ?= ^BenchmarkWrite_
FUZZ_TIME ?= 10s
BCE_FILTER ?= ^\./streamer.*\.go:
COVERAGE_OUT_DIR ?= .coverage
COVERAGE_PROFILE ?= $(COVERAGE_OUT_DIR)/coverage.out
COVERAGE_FUNC ?= $(COVERAGE_OUT_DIR)/coverage.txt
COVERAGE_HTML ?= $(COVERAGE_OUT_DIR)/coverage.html

IPSTREAM_BIN := ipstream
PROFILE_PKG ?= .
PROFILE_BENCH ?= .
PROFILE_RUN_ID := $(shell date +"%Y%m%d_%H%M%S")
PROFILE_OUT_BASE_DIR ?= .profiles
PROFILE_OUT_DIR ?= $(PROFILE_OUT_BASE_DIR)/$(PROFILE_RUN_ID)
PROFILE_OUT_PREFIX ?= $(PROFILE_OUT_DIR)/ipstream_cpu
PROFILE_PPROF := $(PROFILE_OUT_PREFIX).pprof
PROFILE_TOP := $(PROFILE_OUT_PREFIX)_top.txt
PROFILE_LINES := $(PROFILE_OUT_PREFIX)_lines.txt
PROFILE_DOT := $(PROFILE_OUT_PREFIX).dot
PROFILE_SVG := $(PROFILE_OUT_PREFIX).svg
PROFILE_BENCH_OUTPUT := $(PROFILE_OUT_PREFIX)_bench.txt
PROFILE_BENCHSTAT := $(PROFILE_OUT_PREFIX)_benchstat.txt
PROFILE_ALL_FUNCS_LINES := $(PROFILE_OUT_PREFIX)_allfuncs_list.txt
PROFILE_LIST_REGEX ?= github.com/kibaamor/ipstream\..*

check: clean vet lint build test fuzz
	@echo "All checks passed."

clean:
	go clean -testcache
	rm -f $(IPSTREAM_BIN)
	rm -rf $(COVERAGE_OUT_DIR)

vet:
	go vet ./...

lint:
	golangci-lint run ./...

lint-fix:
	golangci-lint run --fix ./...

build:
	go build ./...
	go build -ldflags "$(LDFLAGS)" -o $(IPSTREAM_BIN) ./cmd/ipstream

test:
	go test ./...

bench:
	go test -run '^$$' -bench . -benchmem -benchtime=$(BENCH_TIME) ./...

bench-parse-stats:
	go test $(STATS_BUILD_FLAGS) -run '^$$' -bench '$(PARSE_STATS_BENCH)' -benchmem -benchtime=$(BENCH_TIME) .

fuzz:
	go test -run='^$$' -fuzz=FuzzParseIPv4Fast -fuzztime=$(FUZZ_TIME) .
	go test -run='^$$' -fuzz=FuzzStreamerWrite -fuzztime=$(FUZZ_TIME) .

bce:
	go test -gcflags='all=-d=ssa/check_bce/debug=1' ./... 2>&1 >/dev/null | grep -E '$(BCE_FILTER)' || true

coverage:
	mkdir -p $(COVERAGE_OUT_DIR)
	go test $(STATS_BUILD_FLAGS) -covermode=atomic -coverprofile $(COVERAGE_PROFILE) ./...
	go tool cover -func $(COVERAGE_PROFILE) | tee $(COVERAGE_FUNC)
	go tool cover -html $(COVERAGE_PROFILE) -o $(COVERAGE_HTML)
	@echo "Generated:"
	@echo "  $(COVERAGE_PROFILE)"
	@echo "  $(COVERAGE_FUNC)"
	@echo "  $(COVERAGE_HTML)"

profile-cpu:
	mkdir -p $(PROFILE_OUT_DIR)
	bash -o pipefail -c "go test -run '^$$' -bench '$(PROFILE_BENCH)' -benchmem -benchtime=$(BENCH_TIME) -cpuprofile $(PROFILE_PPROF) $(PROFILE_PKG) | tee $(PROFILE_BENCH_OUTPUT)"

profile-cpu-reports: profile-cpu
	go tool pprof -top $(PROFILE_PPROF) > $(PROFILE_TOP)
	go tool pprof -lines -text $(PROFILE_PPROF) > $(PROFILE_LINES)
	go tool pprof -dot $(PROFILE_PPROF) > $(PROFILE_DOT)
	dot -Tsvg $(PROFILE_DOT) -o $(PROFILE_SVG)
	go tool pprof -list '$(PROFILE_LIST_REGEX)' $(PROFILE_PPROF) > $(PROFILE_ALL_FUNCS_LINES)
	@prev=$$(find $(PROFILE_OUT_BASE_DIR) -mindepth 1 -maxdepth 1 -type d ! -path '$(PROFILE_OUT_DIR)' | sort | tail -n 1); \
	if [ -n "$$prev" ] && [ -f "$$prev/ipstream_cpu_bench.txt" ]; then \
		if command -v benchstat >/dev/null 2>&1; then \
			benchstat "$$prev/ipstream_cpu_bench.txt" "$(PROFILE_BENCH_OUTPUT)" | tee "$(PROFILE_BENCHSTAT)"; \
		else \
			echo "benchstat not found; skipping benchmark comparison"; \
		fi; \
	else \
		echo "No previous profile benchmark output found; skipping benchmark comparison"; \
	fi
	@echo "Generated:"
	@echo "  $(PROFILE_PPROF)"
	@echo "  $(PROFILE_BENCH_OUTPUT)"
	@echo "  $(PROFILE_TOP)"
	@echo "  $(PROFILE_LINES)"
	@echo "  $(PROFILE_DOT)"
	@echo "  $(PROFILE_SVG)"
	@echo "  $(PROFILE_ALL_FUNCS_LINES)"
	@if [ -f "$(PROFILE_BENCHSTAT)" ]; then echo "  $(PROFILE_BENCHSTAT)"; fi
