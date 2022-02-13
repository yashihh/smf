FROM alpine:3.13

LABEL description="free5GC SMF service" version="Stage 3"

ENV F5GC_MODULE smf
ARG DEBUG_TOOLS

# Install debug tools ~ 100MB (if DEBUG_TOOLS is set to true)
RUN if [ "$DEBUG_TOOLS" = "true" ] ; then apk add -U vim strace net-tools curl netcat-openbsd ; fi

Run addgroup -S free5gc && adduser -S free5gc
Run mkdir -p /free5gc && chown -R free5gc:free5gc /free5gc
USER free5gc

# Set working dir
WORKDIR /free5gc
RUN mkdir -p config/TLS/ log/ bin/

# Copy executable
COPY build/bin/${F5GC_MODULE} ./bin/

# Config files volume
VOLUME [ "/free5gc/config" ]

# Certificates (if not using default) volume
VOLUME [ "/free5gc/config/TLS" ]

# Exposed ports
EXPOSE 8000
