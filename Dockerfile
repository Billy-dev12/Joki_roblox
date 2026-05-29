# ========================================================
# STAGE 1: Build static assets Next.js
# ========================================================
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend

# Copy dependencies manifest
COPY frontend/package*.json ./
RUN npm ci

# Copy source files
COPY frontend/ ./

# Buat build statis Next.js (output: export)
ENV NEXT_PUBLIC_API_URL=""
RUN npm run build

# ========================================================
# STAGE 2: Build executable Go Server (CGO-free)
# ========================================================
FROM golang:1.26-alpine AS backend-builder
WORKDIR /app

# Download Go dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy Go server source code
COPY . .

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o joki-server cmd/server/main.go

# ========================================================
# STAGE 3: Final minimal runtime image
# ========================================================
FROM alpine:3.19
WORKDIR /app

# Install ca-certificates (wajib untuk secure HTTPS Roblox API calls dari dalam container)
RUN apk --no-cache add ca-certificates

# Salin binary Go dari Stage 2
COPY --from=backend-builder /app/joki-server .

# Salin aset frontend static Next.js dari Stage 1
COPY --from=frontend-builder /app/frontend/out ./frontend/out

# Buat direktori data database dan public uploads
RUN mkdir -p /app/data /app/public/uploads

# Expose internal port Go server
EXPOSE 8080

# Jalankan server
CMD ["./joki-server"]
