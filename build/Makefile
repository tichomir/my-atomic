# Atomic OS Build System
# Usage:
#   make build-daemon     - Build Go binaries
#   make test             - Run all tests
#   make image-base       - Build atomic-linux OCI image
#   make image-agentic    - Build agentic-os OCI image
#   make policy-test      - Run OPA policy tests
#   make policy-fmt       - Format Rego files
#   make qcow2            - Build QEMU disk image (requires bootc-image-builder)
#   make clean            - Remove build artifacts

REGISTRY   ?= ghcr.io
IMAGE_OWNER ?= tichomir
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

ATOMIC_LINUX_IMAGE  := $(REGISTRY)/$(IMAGE_OWNER)/atomic-linux:$(VERSION)
ATOMIC_CLOUD_IMAGE  := $(REGISTRY)/$(IMAGE_OWNER)/atomic-cloud:$(VERSION)
AGENTIC_OS_IMAGE    := $(REGISTRY)/$(IMAGE_OWNER)/agentic-os:$(VERSION)

DAEMON_DIR  := ../daemon
POLICY_DIR  := ../policy
IMAGES_DIR  := ../images
OUTPUT_DIR  := output

GO          := go
OPA         := opa
PODMAN      := podman
COSIGN      := cosign

.PHONY: all build-daemon test image-base image-cloud image-agentic \
        policy-test policy-fmt push sign qcow2 ami clean help

all: test image-base image-agentic

# --- Go daemon ---

build-daemon:
	@echo "==> Building atomicagentd and atomic-agent-ctl..."
	cd $(DAEMON_DIR) && $(GO) build \
		-ldflags="-s -w -X main.version=$(VERSION)" \
		-o $(CURDIR)/$(OUTPUT_DIR)/atomicagentd \
		./cmd/atomicagentd
	cd $(DAEMON_DIR) && $(GO) build \
		-ldflags="-s -w -X main.version=$(VERSION)" \
		-o $(CURDIR)/$(OUTPUT_DIR)/atomic-agent-ctl \
		./cmd/atomic-agent-ctl
	@echo "==> Binaries written to $(OUTPUT_DIR)/"

test-daemon:
	@echo "==> Running daemon tests..."
	cd $(DAEMON_DIR) && $(GO) test -v -race ./...

# --- OPA policies ---

policy-test:
	@echo "==> Running OPA policy tests..."
	$(OPA) test $(POLICY_DIR)/ -v

policy-fmt:
	@echo "==> Formatting Rego files..."
	$(OPA) fmt --write $(POLICY_DIR)/

policy-check:
	@echo "==> Checking Rego formatting (no changes)..."
	$(OPA) fmt --diff $(POLICY_DIR)/

# --- Combined test target ---

test: test-daemon policy-test
	@echo "==> All tests passed"

# --- OCI image builds ---

image-base:
	@echo "==> Building $(ATOMIC_LINUX_IMAGE)..."
	$(PODMAN) build \
		--file $(IMAGES_DIR)/atomic-base/Containerfile \
		--tag $(ATOMIC_LINUX_IMAGE) \
		--build-arg BUILD_VERSION=$(VERSION) \
		..
	@echo "==> Built $(ATOMIC_LINUX_IMAGE)"

image-cloud:
	@echo "==> Building $(ATOMIC_CLOUD_IMAGE)..."
	$(PODMAN) build \
		--file $(IMAGES_DIR)/atomic-cloud/Containerfile \
		--tag $(ATOMIC_CLOUD_IMAGE) \
		--build-arg BUILD_VERSION=$(VERSION) \
		--build-arg BASE_IMAGE=$(ATOMIC_LINUX_IMAGE) \
		..
	@echo "==> Built $(ATOMIC_CLOUD_IMAGE)"

image-agentic: image-base image-cloud
	@echo "==> Building $(AGENTIC_OS_IMAGE)..."
	$(PODMAN) build \
		--file $(IMAGES_DIR)/agentic-os/Containerfile \
		--tag $(AGENTIC_OS_IMAGE) \
		--build-arg BUILD_VERSION=$(VERSION) \
		--from $(ATOMIC_CLOUD_IMAGE) \
		..
	@echo "==> Built $(AGENTIC_OS_IMAGE)"

# --- Push and sign ---

push: image-agentic
	@echo "==> Pushing images to $(REGISTRY)..."
	$(PODMAN) push $(ATOMIC_LINUX_IMAGE)
	$(PODMAN) push $(ATOMIC_CLOUD_IMAGE)
	$(PODMAN) push $(AGENTIC_OS_IMAGE)

sign: push
	@echo "==> Signing $(AGENTIC_OS_IMAGE) with cosign..."
	$(COSIGN) sign --yes $(AGENTIC_OS_IMAGE)

# --- Disk image builds (requires bootc-image-builder) ---

$(OUTPUT_DIR):
	mkdir -p $(OUTPUT_DIR)

qcow2: $(OUTPUT_DIR)
	@echo "==> Building QEMU disk image from $(AGENTIC_OS_IMAGE)..."
	sudo $(PODMAN) run \
		--rm \
		--privileged \
		-v $(CURDIR)/$(OUTPUT_DIR):/output \
		quay.io/centos-bootc/bootc-image-builder:latest \
		--type qcow2 \
		--output /output \
		$(AGENTIC_OS_IMAGE)
	@echo "==> QEMU image: $(OUTPUT_DIR)/qcow2/disk.qcow2"

ami: $(OUTPUT_DIR)
	@echo "==> Building AWS AMI from $(AGENTIC_OS_IMAGE)..."
	@echo "==> Requires AWS credentials in environment"
	sudo $(PODMAN) run \
		--rm \
		--privileged \
		-v $(CURDIR)/$(OUTPUT_DIR):/output \
		-e AWS_ACCESS_KEY_ID \
		-e AWS_SECRET_ACCESS_KEY \
		-e AWS_DEFAULT_REGION \
		quay.io/centos-bootc/bootc-image-builder:latest \
		--type ami \
		--output /output \
		$(AGENTIC_OS_IMAGE)

# --- Local testing ---

run-vm: qcow2
	@echo "==> Starting QEMU VM with Agentic OS..."
	qemu-system-x86_64 \
		-m 2048 \
		-smp 2 \
		-drive file=$(OUTPUT_DIR)/qcow2/disk.qcow2,if=virtio \
		-net user,hostfwd=tcp::2222-:22 \
		-net nic \
		-nographic

# --- Cleanup ---

clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf $(OUTPUT_DIR)
	$(PODMAN) rmi $(ATOMIC_LINUX_IMAGE) $(ATOMIC_CLOUD_IMAGE) $(AGENTIC_OS_IMAGE) 2>/dev/null || true

# --- Help ---

help:
	@echo "Atomic OS Build Targets:"
	@echo ""
	@echo "  build-daemon    Build atomicagentd and atomic-agent-ctl binaries"
	@echo "  test            Run all tests (daemon + policies)"
	@echo "  test-daemon     Run Go daemon tests"
	@echo "  policy-test     Run OPA policy unit tests"
	@echo "  policy-fmt      Format Rego policy files"
	@echo "  image-base      Build atomic-linux OCI image"
	@echo "  image-cloud     Build atomic-cloud OCI image"
	@echo "  image-agentic   Build agentic-os OCI image (full stack)"
	@echo "  push            Push all images to registry"
	@echo "  sign            Sign images with cosign"
	@echo "  qcow2           Build QEMU disk image"
	@echo "  ami             Build AWS AMI"
	@echo "  run-vm          Start a local QEMU VM for testing"
	@echo "  clean           Remove build artifacts and images"
	@echo ""
	@echo "Variables:"
	@echo "  REGISTRY=$(REGISTRY)"
	@echo "  IMAGE_OWNER=$(IMAGE_OWNER)"
	@echo "  VERSION=$(VERSION)"
