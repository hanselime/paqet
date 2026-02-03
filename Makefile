# paqet Makefile
# Android build requires ANDROID_NDK_HOME (or ANDROID_NDK_ROOT) and CGO.

.PHONY: android android-arm64 android-arm clean-android help

# Version / ref for ldflags (optional)
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
GIT_TAG ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d %H:%M:%S UTC')

# Android NDK: prefer ANDROID_NDK_HOME, then ANDROID_NDK_ROOT
ANDROID_NDK ?= $(firstword $(ANDROID_NDK_HOME) $(ANDROID_NDK_ROOT))
BUILD_DIR := build/android
LIBPCAP_ARM64 := $(BUILD_DIR)/libpcap/arm64-v8a
LIBPCAP_ARM   := $(BUILD_DIR)/libpcap/armeabi-v7a

# Host tag for NDK prebuilt (linux-x86_64, darwin-arm64, windows-x86_64, etc.)
UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)
ifeq ($(UNAME_S),Linux)
  NDK_HOST := linux-x86_64
  NDK_EXE :=
else ifeq ($(UNAME_S),Darwin)
  ifeq ($(UNAME_M),arm64)
    NDK_HOST := darwin-arm64
  else
    NDK_HOST := darwin-x86_64
  endif
  NDK_EXE :=
else ifneq (,$(filter MINGW% MSYS% CYGWIN%,$(UNAME_S)))
  NDK_HOST := windows-x86_64
  NDK_EXE := .exe
else
  NDK_HOST := linux-x86_64
  NDK_EXE :=
endif

NDK_BIN := $(ANDROID_NDK)/toolchains/llvm/prebuilt/$(NDK_HOST)/bin
ANDROID_API ?= 21

help:
	@echo "Android build (rooted device):"
	@echo "  make android        - Build for arm64 and arm (default: arm64 only if one ABI)"
	@echo "  make android-arm64  - Build paqet for Android arm64-v8a"
	@echo "  make android-arm    - Build paqet for Android armeabi-v7a"
	@echo ""
	@echo "Set ANDROID_NDK_HOME or ANDROID_NDK_ROOT to your NDK path."
	@echo "Requires: Go with CGO, NDK, and for libpcap: flex, bison, autoconf, automake, libtool."

# Build both ABIs
android: android-arm64 android-arm

# --- arm64-v8a ---
android-arm64: $(LIBPCAP_ARM64)/lib/libpcap.a
	@mkdir -p $(BUILD_DIR)
	@echo "Building paqet for android/arm64..."
	$(if $(ANDROID_NDK),,$(error ANDROID_NDK_HOME or ANDROID_NDK_ROOT must be set))
	GOOS=android GOARCH=arm64 CGO_ENABLED=1 \
	CC="$(NDK_BIN)/aarch64-linux-android$(ANDROID_API)-clang$(NDK_EXE)" \
	CGO_CFLAGS="-I$(abspath $(LIBPCAP_ARM64)/include)" \
	CGO_LDFLAGS="-L$(abspath $(LIBPCAP_ARM64)/lib) -lpcap" \
	go build -trimpath \
		-ldflags "-s -w -X 'paqet/cmd/version.Version=$(VERSION)' -X 'paqet/cmd/version.GitCommit=$(GIT_COMMIT)' -X 'paqet/cmd/version.GitTag=$(GIT_TAG)' -X 'paqet/cmd/version.BuildTime=$(BUILD_TIME)'" \
		-o $(BUILD_DIR)/paqet_android_arm64 ./cmd/main.go
	@echo "Built: $(BUILD_DIR)/paqet_android_arm64"

# --- armeabi-v7a ---
# Use -mfloat-abi=softfp and -marm so Go output matches libpcap (armelf_linux_eabi); avoids "incompatible with armelf_linux_eabi" linker errors.
android-arm: $(LIBPCAP_ARM)/lib/libpcap.a
	@mkdir -p $(BUILD_DIR)
	@echo "Building paqet for android/arm..."
	$(if $(ANDROID_NDK),,$(error ANDROID_NDK_HOME or ANDROID_NDK_ROOT must be set))
	GOOS=android GOARCH=arm GOARM=7 CGO_ENABLED=1 \
	CC="$(NDK_BIN)/armv7a-linux-androideabi$(ANDROID_API)-clang$(NDK_EXE)" \
	CGO_CFLAGS="-I$(abspath $(LIBPCAP_ARM)/include) -mfloat-abi=softfp -marm" \
	CGO_LDFLAGS="-L$(abspath $(LIBPCAP_ARM)/lib) -lpcap -mfloat-abi=softfp -marm" \
	go build -trimpath \
		-ldflags "-s -w -X 'paqet/cmd/version.Version=$(VERSION)' -X 'paqet/cmd/version.GitCommit=$(GIT_COMMIT)' -X 'paqet/cmd/version.GitTag=$(GIT_TAG)' -X 'paqet/cmd/version.BuildTime=$(BUILD_TIME)'" \
		-o $(BUILD_DIR)/paqet_android_arm ./cmd/main.go
	@echo "Built: $(BUILD_DIR)/paqet_android_arm"

# Build libpcap for arm64-v8a
$(LIBPCAP_ARM64)/lib/libpcap.a:
	$(if $(ANDROID_NDK),,$(error ANDROID_NDK_HOME or ANDROID_NDK_ROOT must be set))
	ANDROID_NDK_HOME="$(ANDROID_NDK)" bash "$(CURDIR)/scripts/build-libpcap-android.sh" arm64-v8a

# Build libpcap for armeabi-v7a
$(LIBPCAP_ARM)/lib/libpcap.a:
	$(if $(ANDROID_NDK),,$(error ANDROID_NDK_HOME or ANDROID_NDK_ROOT must be set))
	ANDROID_NDK_HOME="$(ANDROID_NDK)" bash "$(CURDIR)/scripts/build-libpcap-android.sh" armeabi-v7a

clean-android:
	rm -rf $(BUILD_DIR)
