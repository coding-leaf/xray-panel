# Execution Plan (Parent Task)

## Milestone Order

1. **Sub-1 (`11-03-sub-1-backend-xray-schema-and-mux-compiler`)**:
   - Activate Sub-1 -> Dispatch `trellis-implement` (`magpie/lynnsora/gemini-3.8-flash`) -> Run backend quality gates & `./temp/xray.exe` tests -> Dispatch `trellis-check` -> Archive Sub-1.
2. **Sub-2 (`11-03-sub-2-frontend-outbound-and-topology-mux`)**:
   - Activate Sub-2 -> Dispatch `trellis-implement` (`magpie/lynnsora/gemini-3.8-flash`) -> Run frontend build & typecheck gate -> Dispatch `trellis-check` -> Archive Sub-2.
3. **Sub-3 (`11-03-sub-3-frontend-inbound-and-routing-alignment`)**:
   - Activate Sub-3 -> Dispatch `trellis-implement` (`magpie/lynnsora/gemini-3.8-flash`) -> Run frontend + backend full quality gates -> Dispatch `trellis-check` -> Archive Sub-3.
4. **Parent Final Integration & Spec Update**:
   - Run full repository quality gates (`go test ./...`, `go vet ./...`, `go build .`, `npm run build`).
   - Update `.trellis/spec/` with newly established Xray schema preservation contracts.
