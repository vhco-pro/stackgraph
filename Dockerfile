FROM alpine:3.23

# Build-time variables
# IMAGE_NAME is passed from docker-release-action (the 'project' parameter)
ARG IMAGE_NAME
# TARGETARCH and TARGETOS are automatically set by Docker buildx
ARG TARGETARCH
ARG TARGETOS

# Copy the ARG value to an ENV variable that will persist at runtime
ENV IMAGE_NAME=${IMAGE_NAME}

# Install runtime dependencies
RUN apk add --no-cache git ca-certificates wget

# Create a non-root user with UID 1001 to match GitHub Actions runner
RUN addgroup -g 1001 ${IMAGE_NAME} && \
    adduser -D -u 1001 -G ${IMAGE_NAME} ${IMAGE_NAME}

# Configure git to trust any directory (safe for container use)
# This must be done as root before switching to non-root user
RUN git config --system --add safe.directory '*'

# Copy the pre-built binary from GoReleaser's dist directory
# GoReleaser creates binaries in dist/${IMAGE_NAME}_${TARGETOS}_${TARGETARCH}_v1/ for amd64
# and dist/${IMAGE_NAME}_${TARGETOS}_${TARGETARCH}/ for arm64
COPY dist/${IMAGE_NAME}_${TARGETOS}_${TARGETARCH}*/${IMAGE_NAME} /usr/local/bin/${IMAGE_NAME}

# Set proper ownership and permissions
RUN chmod +x /usr/local/bin/${IMAGE_NAME} && \
    chown ${IMAGE_NAME}:${IMAGE_NAME} /usr/local/bin/${IMAGE_NAME}

# Switch to the non-root user & set working directory
USER ${IMAGE_NAME}
WORKDIR /home/${IMAGE_NAME}

# Use exec form with environment variable substitution
ENTRYPOINT ["/bin/sh", "-c", "/usr/local/bin/${IMAGE_NAME}"]