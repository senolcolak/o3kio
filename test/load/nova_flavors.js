// k6 load testing script for Nova flavors API
// Run: k6 run test/load/nova_flavors.js

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics
const flavorListSuccessRate = new Rate('flavor_list_success_rate');
const flavorDetailSuccessRate = new Rate('flavor_detail_success_rate');
const flavorListDuration = new Trend('flavor_list_duration');
const flavorDetailDuration = new Trend('flavor_detail_duration');

// Test configuration
export const options = {
  stages: [
    { duration: '30s', target: 50 },    // Ramp up to 50 req/s
    { duration: '1m', target: 50 },     // Baseline
    { duration: '30s', target: 200 },   // Ramp up to 200 req/s
    { duration: '1m', target: 200 },    // Sustained load
    { duration: '30s', target: 500 },   // Sprint 69 target: 500 req/s
    { duration: '1m', target: 500 },    // Sustained at target
    { duration: '30s', target: 0 },     // Ramp down
  ],
  thresholds: {
    'http_req_duration': ['p(95)<5'],          // 95% under 5ms
    'flavor_list_success_rate': ['rate>0.99'], // 99% success
    'flavor_detail_success_rate': ['rate>0.99'],
  },
};

const BASE_URL = __ENV.O3K_URL || 'http://localhost:8774';
const KEYSTONE_URL = __ENV.KEYSTONE_URL || 'http://localhost:35357';

// Get auth token (run once per VU)
function getAuthToken() {
  const authPayload = JSON.stringify({
    auth: {
      identity: {
        methods: ['password'],
        password: {
          user: {
            name: 'admin',
            domain: { name: 'Default' },
            password: 'secret',
          },
        },
      },
      scope: {
        project: {
          name: 'default',
          domain: { name: 'Default' },
        },
      },
    },
  });

  const authResponse = http.post(
    `${KEYSTONE_URL}/v3/auth/tokens`,
    authPayload,
    { headers: { 'Content-Type': 'application/json' } }
  );

  return authResponse.headers['X-Subject-Token'];
}

export function setup() {
  // Get auth token for all VUs to use
  const token = getAuthToken();
  return { token };
}

export default function (data) {
  const token = data.token;
  const headers = {
    'X-Auth-Token': token,
    'Content-Type': 'application/json',
  };

  // List flavors (cached, should be fast)
  const listStart = Date.now();
  const listResponse = http.get(`${BASE_URL}/v2.1/flavors/detail`, { headers });
  const listDuration = Date.now() - listStart;

  const listSuccess = check(listResponse, {
    'list: status is 200': (r) => r.status === 200,
    'list: has flavors': (r) => {
      const body = JSON.parse(r.body);
      return body.flavors && Array.isArray(body.flavors);
    },
  });

  flavorListSuccessRate.add(listSuccess);
  flavorListDuration.add(listDuration);

  // Get flavor detail (cached)
  if (listSuccess && listResponse.body) {
    const body = JSON.parse(listResponse.body);
    if (body.flavors && body.flavors.length > 0) {
      const flavorId = body.flavors[0].id;

      const detailStart = Date.now();
      const detailResponse = http.get(
        `${BASE_URL}/v2.1/flavors/${flavorId}`,
        { headers }
      );
      const detailDuration = Date.now() - detailStart;

      const detailSuccess = check(detailResponse, {
        'detail: status is 200': (r) => r.status === 200,
        'detail: has flavor': (r) => {
          const body = JSON.parse(r.body);
          return body.flavor && body.flavor.id === flavorId;
        },
      });

      flavorDetailSuccessRate.add(detailSuccess);
      flavorDetailDuration.add(detailDuration);
    }
  }

  sleep(0.1); // 100ms think time
}
