// k6 version of `make loadtest` (same five endpoints, p95 < 300 ms). Fill the data first with
//   go run ./tools/loadtest -n 1        (adds 50k signals + 20k chat messages, dev only)
// then: k6 run -e BASE=http://127.0.0.1:8080 tools/loadtest/k6.js
import http from 'k6/http'
import { check } from 'k6'

const BASE = __ENV.BASE || 'http://127.0.0.1:8080'
const USER = __ENV.ARC_USER || 'sam@gsi.co.id'
const paths = ['/api/orbit', '/api/segmen', '/api/relasi', '/api/dealers/sinar', '/api/dealers/due']

export const options = {
  vus: 8,
  duration: '60s',
  thresholds: Object.fromEntries(paths.map((p) => [`http_req_duration{path:${p}}`, ['p(95)<300']])),
}

export default function () {
  for (const p of paths) {
    const r = http.get(BASE + p, { headers: { 'X-Dev-User': USER }, tags: { path: p } })
    check(r, { 'status 200': (x) => x.status === 200 })
  }
}
