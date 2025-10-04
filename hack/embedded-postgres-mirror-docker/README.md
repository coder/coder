# PostgreSQL Binary Mirror

This directory contains a Docker-based mirror for PostgreSQL binaries used by `fergusstrange/embedded-postgres`. It eliminates the external dependency on Maven repositories that causes test flakes in CI.

## Purpose

The `fergusstrange/embedded-postgres` library downloads PostgreSQL binaries from Maven repositories like `repo1.maven.org/maven2`. When these repositories are unreachable, tests fail with network errors. This mirror provides a reliable, self-hosted alternative.

## How it works

1. **Pre-download**: The `download-postgres-binaries.sh` script downloads all required PostgreSQL binaries from Maven repositories and stores them in the `files/` directory with the correct directory structure.

2. **Docker image**: The Dockerfile creates a lightweight nginx-based server that serves the binaries using the same URL structure that embedded-postgres expects.

3. **Deployment**: The image can be deployed to Cloud Run, providing a globally available, reliable binary server.

## Usage

### 1. Download binaries

```bash
cd hack/embedded-postgres-mirror-docker
./download-postgres-binaries.sh
```

This creates a `files/` directory with the Maven repository structure:

```
files/
└── io/zonky/test/postgres/
    ├── embedded-postgres-binaries-linux-amd64/
    │   ├── 13.14.0/
    │   │   ├── embedded-postgres-binaries-linux-amd64-13.14.0.jar
    │   │   └── embedded-postgres-binaries-linux-amd64-13.14.0.jar.sha256
    │   └── 16.6.0/
    │       ├── embedded-postgres-binaries-linux-amd64-16.6.0.jar
    │       └── embedded-postgres-binaries-linux-amd64-16.6.0.jar.sha256
    └── ... (other platforms)
```

### 2. Build Docker image

```bash
docker build -t postgres-binary-mirror .
```

### 3. Test locally

```bash
docker run -p 8080:8080 postgres-binary-mirror
```

Visit `http://localhost:8080/` to browse the repository structure.

Test a specific binary:
```bash
curl -I http://localhost:8080/io/zonky/test/postgres/embedded-postgres-binaries-linux-amd64/13.14.0/embedded-postgres-binaries-linux-amd64-13.14.0.jar
```

### 4. Deploy to Cloud Run

```bash
# Tag for registry
docker tag postgres-binary-mirror gcr.io/YOUR_PROJECT/postgres-binary-mirror

# Push to registry  
docker push gcr.io/YOUR_PROJECT/postgres-binary-mirror

# Deploy to Cloud Run
gcloud run deploy postgres-binary-mirror \
  --image gcr.io/YOUR_PROJECT/postgres-binary-mirror \
  --platform managed \
  --region us-central1 \
  --allow-unauthenticated \
  --port 8080
```

## Configuration

### Supported PostgreSQL versions

Currently downloads:
- **13.14.0** (used in `cli/server.go`)  
- **16.6.0** (used in `scripts/embedded-pg/main.go`)

### Supported platforms

- `linux-amd64`
- `darwin-amd64`
- `darwin-arm64v8` 
- `windows-amd64`

To add more versions or platforms, edit the arrays in `download-postgres-binaries.sh`.

## Integration with Coder

Once deployed, update the Coder configuration to use your mirror:

```go
// In cli/server.go, replace:
BinaryRepositoryURL("https://repo.maven.apache.org/maven2")

// With:
BinaryRepositoryURL("https://your-postgres-binaries.run.app")
```

## Files

- **`download-postgres-binaries.sh`** - Downloads binaries from Maven repositories
- **`Dockerfile`** - Creates the nginx-based binary server
- **`nginx.conf`** - Nginx configuration with proper MIME types and caching
- **`files/`** - Directory created by download script (not checked into git)

## Benefits

- **Eliminates network flakes**: No dependency on external Maven repositories
- **Fast and reliable**: Globally distributed via Cloud Run
- **Cost effective**: Pay only for actual usage
- **Maintenance free**: Binaries rarely change
- **Easy updates**: Re-run download script to add new versions