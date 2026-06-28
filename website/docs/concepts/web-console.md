# Web Console

azemu ships an embedded web console that gives you a local "Azure Portal"
for your emulator. It runs on port **4570** (configurable via
`AZEMU_CONSOLE_PORT`) and is served directly from the azemu binary with
no extra dependencies.

Open `http://localhost:4570` after starting azemu.

## Views

The console has three composable views, each modelled after a real Azure
Portal interaction pattern.

### Cockpit (Home)

The default view at `/`. A dashboard showing:

- **Service cards** for each azemu port (ARM :4566, Metadata :4567,
  Health :4568, ADO OIDC :4569) with live health status
- **Meta strip** showing version, uptime, resource count, store type,
  and TLS algorithm
- **Resource inventory** tiles grouped by ARM resource type, with
  Export, Import, and Reset controls
- **Live request log** streaming ARM requests in real time via SSE

### Portal Classic

A blade-style view at `/resource-groups/:name`, modelled after the Azure
Portal resource group blade:

- Breadcrumb navigation (Home / Resource groups / name)
- Command bar (Create, Refresh, Export template, Delete)
- Essentials panel (subscription, location, provisioning state)
- Resources table with type badges, location, status, and creation time

### IDE Explorer

A three-pane layout at `/explorer`, modelled after VS Code:

- **Side nav** with service categories and emulator tools
- **Resource tree** grouping resources by resource group, with
  expand/collapse and selection
- **Detail blade** with tabbed content (Overview, Secrets, Keys,
  Access, JSON) and an essentials grid
- **Docked log** panel at the bottom with tabs for Request log,
  Events, and State store

## Live request streaming

The console connects to `GET /api/requests/stream` (SSE) and displays
ARM requests as they arrive. Each entry shows:

- Timestamp, HTTP method, path, status code, and duration in
  milliseconds
- Method and status are color-coded (GET green, PUT orange, DELETE red;
  2xx green, 4xx amber, 5xx red)

The backend keeps a ring buffer of the last 500 requests. On connect,
the SSE endpoint sends a backfill of recent entries, then streams new
ones as they happen.

## Building the console

The console SPA source lives in `console/` (React, Vite, TypeScript).
The built assets are embedded into the Go binary via `internal/console/`.

```bash
make console-build   # npm ci + build + copy to internal/console/dist
make build           # go build with embedded assets
```

For development with hot reload:

```bash
make console-dev     # starts Vite dev server on :5173 with proxy to azemu
```

## Configuration

| Variable | Default | Purpose |
|----------|---------|---------|
| `AZEMU_CONSOLE_PORT` | `4570` | HTTP port for the web console |
