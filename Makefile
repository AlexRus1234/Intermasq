# Intermasq - Web panel for dnsmasq
# Copyright (C) 2026 AlexRus1234
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU Affero General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# GNU Affero General Public License for more details.
#
# You should have received a copy of the GNU Affero General Public License
# along with this program. If not, see <https://www.gnu.org/licenses/>.

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
