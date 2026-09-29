# Research — Frontend Conventions (read-only, 2026-09-29)

Scope: `web/`. All paths repo-root relative. Evidence is `file:line`.

## 1. `web/src/` structure

Only 6 subdirectories; **no** `stores/`, `composables/`, `types/`, `assets/`.

| Path | Contents |
|---|---|
| `web/src/views/` | 12 views, all `*View.vue` (PascalCase) |
| `web/src/components/` | Exactly one shared component: `ToastContainer.vue` |
| `web/src/api/` | `index.ts` (axios wrapper), `reality.ts` (typed domain API) |
| `web/src/router/` | Single `index.ts` |
| `web/src/mock/` | `index.ts` (dispatcher, 922 lines), `storage.ts` (localStorage state, 567 lines) — `mock/index.ts:41`, `mock/storage.ts:3` |
| `web/src/utils/` | Only `toast.ts` |
| `web/src/` root | `App.vue`, `main.ts`, `style.css`, `vite-env.d.ts` |

Views (lines): Inbounds 1602, Users 1285, Logs 1146, Outbounds 859, Topology 817,
Routing 730, Settings 517, PortalClaim 351, Dashboard 346, DNS 284, Login 229, Config 189.

## 2. Router — `web/src/router/index.ts`

- Single file; lazy imports as consts — `:4-15`; flat table, no `children` — `:17-31`.
- Paths: `/login`, `/portal`, `/`, `/topology`, `/inbounds`, `/outbounds`, `/routing`,
  `/dns`, `/users`, `/config`, `/logs`, `/settings`, catch-all redirect `:30`.
- Global `router.beforeEach` — `:38-50`: token from `localStorage.getItem('token')` `:42`;
  `requiresAuth` → `/login` `:43-44`; mock mode auto-seeds token `:39-41`;
  logged-in bounced off `/login` `:45-46`.
- `meta.layout: 'blank'` for `/login`, `/portal` `:18-19`; consumed `App.vue:338-341`.
- `afterEach` sets `document.title` `:52-60`. History base via `(import.meta as any).env?.BASE_URL` `:34`.

## 3. State management — Pinia unused

- Pinia registered `web/src/main.ts:2,8`; **zero stores** (`defineStore` grep = 0).
- All state is component-local `ref`/`computed` (e.g. `InboundsView.vue:925-995`).
- Cross-cutting state = module-level `ref` in `web/src/utils/toast.ts:12`.
- Auth state in `localStorage` keys `token`/`username`, read ad hoc
  (`router/index.ts:39-42`, `api/index.ts:10,21`, `LoginView.vue:235-236`, `App.vue:342,443-444`).

## 4. API layer — `web/src/api/index.ts`

- axios singleton `:4-7`: `baseURL: '/api'`, `timeout: 30000`.
- Request interceptor `:9-15`: `Authorization = Bearer ${token}` from localStorage `:10-13`.
- Response interceptor `:17-28`: returns `response.data` `:18`; on **401** clears token and
  hard-redirects to `/login` (skips `/login`, `/portal`) `:20-25`; else rejects with
  `error.response?.data?.error || error.message` `:26`.
- Generic wrappers `api.get<T>/post<T>/put<T>/delete<T>` `:30-55`, each routes to
  `handleMockRequest` when `isMockMode()`.
- Default export `api`, re-exports `./reality` `:57-58`.
- Typing weak: `<T = any>`, views use `ref<any>` (e.g. `DashboardView.vue:278`).
  Only `web/src/api/reality.ts` is typed: `RealityDomainStatus`/`RealityCheckItem`/
  `RealitySummaryStatus` `:3-30`, `getRealityStatus()` `:32-38`, `checkRealityStatus()` `:40-45`
  (defensive `res.data` unwrap `:34,42`).

## 5. SFC conventions

- 14/14 `.vue` use `<script setup lang="ts">`; no Options API.
- **No** `defineProps` / `defineEmits` / `defineExpose` / `withDefaults` anywhere.
  Composition via module singletons (`toast`, `api`), not prop drilling.
- No custom composables; only `useRoute`/`useRouter` (`App.vue:326-327`,
  `InboundsView.vue:933`, `OutboundsView.vue:500`, `LoginView.vue:201`, `TopologyView.vue:419`).
- Naming: PascalCase files; views suffixed `View.vue`.
- Imports are **relative** (`'../api'`, `'../utils/toast'`); `@/` alias unused
  (grep `from '@/` = 0) though configured.
- Domain types declared inline in components (e.g. `interface SubRouteItem` `InboundsView.vue:916-923`).
- Views have no `<style>` block; only `ToastContainer.vue:67` has `<style scoped>`.
- Lifecycle: `onMounted` setInterval + `onUnmounted` clear + `visibilitychange` — `App.vue:417-440`.

## 6. Styling

- Tailwind + PostCSS; entry `web/src/style.css:1-3`.
- Global design classes in `style.css`: `.glass-panel` `:20-26`, `.glass-card` `:28-41`,
  `.btn-primary` `:44-56`, `.btn-secondary` `:58-68`, `.pulse-green` `:71-88`,
  `.fade-slide-*` `:91-102`, `.modal-scale-*` `:105-113`, custom scrollbars `:116-129`.
- `tailwind.config.js`: content globs `:3-6`, `darkMode: 'class'` `:7`, extended
  `dark`/`brand`/`cyan` palettes `:9-29`, `plugins: []` `:32`.
- **No component library**; all UI hand-rolled Tailwind with raw hex (e.g. `bg-[#07090E]` `App.vue:5,9`).
- Icons: `lucide-vue-next` per-file, `<component :is="item.icon">` (`App.vue:304-320,47-51`).
- Other UI dep: `qrcode.vue` (`UsersView.vue:909`, `SettingsView.vue:380`).
- Fonts from Google Fonts `web/index.html:8-10`; `html class="dark"` `:2`.

## 7. TypeScript & build

`web/package.json:6-13` scripts: `dev` (vite), `dev:demo` (vite --mode demo),
`build` (vite build), `build:demo`, `typecheck` (vue-tsc --noEmit), `preview`.

`web/tsconfig.json`: `strict: false` `:7`; `moduleResolution: "Bundler"` `:6`;
`target/module ESNext` `:3-4`; `noEmit` `:14`; `skipLibCheck` `:13`;
alias `@/* → src/*` `:16-18` (mirrored `vite.config.ts:8-12`) but unused.

`web/vite.config.ts`: `base: './'` `:6`; dev port 5173 `:14`; `/api` proxy → `http://127.0.0.1:9000` `:15-20`;
manual vendor chunk splitting `:26-49`.

## 8. Testing — NONE

- No test files/framework under `web/` (vitest/jest/playwright/cypress = 0 matches).
- No test deps, no `test` script, no config files.
- Only gates: `typecheck` and `build`.

## 9. i18n & env

- No i18n (`vue-i18n` / `useI18n` / `$t(` = 0). UI strings hardcoded Chinese/English.
- Only env file: `web/.env.demo` with `VITE_MOCK_MODE=true` (`:1`).
- Env reads: `VITE_MOCK_MODE === 'true'` `mock/index.ts:10`; `env.MODE === 'demo'` `:11`;
  `?mock=true` query `:12`; `github.io` hostname `:13`; `(import.meta as any).env?.BASE_URL` `router/index.ts:34`.
- No typed `ImportMetaEnv`; `web/src/vite-env.d.ts:1-7` only vite client ref + `*.vue` shim.

## Conventions / gaps to keep in mind

- Flat, monolithic SFCs with no props/emits, no stores, no composables — architecture is intentionally flat.
- Data flow: view → `api` singleton → axios or mock dispatcher; notifications via `toast` singleton.
- Type safety weak by design (`strict:false`, `<T = any>`, pervasive `: any`).
- No frontend test safety net; only `typecheck` + `build`.
- `@/` alias and Pinia are configured/wired but unused in practice.
