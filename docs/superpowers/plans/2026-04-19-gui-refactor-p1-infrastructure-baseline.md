# P1 Infrastructure — Pre-change baseline

Run: `cd gui/web && npm ci && npm run build`
Date: 2026-04-19
Branch: `refactor/gui-v2` from `main` (commit `05a0487`)

## Bundle sizes (bytes, from dist/assets/)

### JS (total ≈ 227.6 KB raw, 76.8 KB gzip)

| File | raw | gzip |
|------|-----|------|
| `index-0gQJro8N.js`               | 97,699 | 34.94 KB |
| `Dashboard-BwG2wWJw.js`           | 28,796 |  9.43 KB |
| `Settings-CEd2pOhg.js`            | 28,328 |  8.31 KB |
| `Routing-Dvv4ipsO.js`             | 19,362 |  6.36 KB |
| `Servers-DKwP__9T.js`             | 10,627 |  3.55 KB |
| `Logs-BRS0SOb4.js`                | 10,623 |  3.57 KB |
| `Mesh-GvgL5uCY.js`                |  6,415 |  2.37 KB |
| `Subscriptions-Dvu4Bd50.js`       |  6,249 |  2.40 KB |
| `Groups-C9gT3JpB.js`              |  5,401 |  2.18 KB |
| `MeshTopologyChart-uup9u9R6.js`   |  4,456 |  1.87 KB |
| misc (select, props, style, this) |  2,541 |  1.60 KB |

### CSS (total ≈ 87.2 KB raw, 17.4 KB gzip)

| File | raw | gzip |
|------|-----|------|
| `Settings-DZomXe5g.css`             | 17,210 | 2.54 KB |
| `index-CJr0QzwM.css`                | 15,577 | 3.32 KB |
| `Dashboard-C3zKTz8T.css`            | 13,637 | 2.62 KB |
| `Routing-DEagN7MB.css`              | 12,252 | 1.95 KB |
| `Servers-Bx541bA0.css`              |  7,163 | 1.51 KB |
| `Subscriptions-BwG9r-62.css`        |  5,277 | — |
| `Logs-CUotOx6r.css`                 |  4,913 | — |
| `Groups-DA049InX.css`               |  4,651 | — |
| `Mesh-DC10q_Nm.css`                 |  3,732 | — |
| `MeshTopologyChart-UZOqd3jK.css`    |  1,531 | — |

### Totals (from `vite build` gzip column)

- JS gzip total: **~76.8 KB**
- CSS gzip total: **~17.4 KB** (incl. lazy-loaded chunks not all reported gzip)
- Combined gzip: **~94.2 KB**

## Budget for P1 PR

Total JS + CSS gzip delta ≤ **+30 KB** (bits-ui + testing-library runtime + ui/ primitives).
If the delta exceeds that we stop and investigate tree-shaking — see Task 43.
