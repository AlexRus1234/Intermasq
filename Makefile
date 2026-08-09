# Intermasq local build helpers.
#
# CI (.forgejo/workflows/build.yml) rebuilds the frontend in its own step
# before `go build`, so the //go:embed frontend/dist/* directive never
# bundles a stale bundle there. Locally a bare `go build` skips that and
# silently embeds whatever frontend/dist last held — `make build` mirrors
# the CI order (npm run build → go build) so local binaries stay honest.
#
# Requires Node/npm on PATH; run `npm install` once in frontend/ (the
# `frontend` target does it automatically when node_modules is missing).

.PHONY: build frontend clean

build: frontend
	go build -o intermasq.exe .

frontend: frontend/node_modules
	cd frontend && npm run build

frontend/node_modules: frontend/package.json frontend/package-lock.json
	cd frontend && npm install

clean:
	rm -rf frontend/dist intermasq.exe
