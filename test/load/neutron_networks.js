// k6 load testing script for Neutron networks API
// Run: k6 run test/load/neutron_networks.js

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics
const networkListSuccessRate = new Rate('network_list_success_rate');
const networkDetailSuccessRate = new Rate('network_detail_success_rate');
const networkListDuration = new Trend('network_list_duration');
const networkDetailDuration = new Trend('network_detail_duration');

// Test configuration
export const options = {
  stages: [
    { duration: '30s', target: 50 },    // Ramp up to 50 req/s
    { duration: '1m', target: 50 },     // Baseline
    { duration: '30s', target: 200 },   // Mid-range
    { duration: '1m', target: 200 },    // Sustained
    { duration: '30s', target: 400 },   // Approaching target
    { duration: '1m', target: 400 },    // Sustained
    { duration: '30s', target: 0 },     // Ramp down
  ],
  thresholds: {
    'http_req_duration': ['p(95)<5'],            // 95% under 5ms
    'network_list_success_rate': ['rate>0.99'],  // 99% success
    'network_detail_success_rate': ['rate>0.99'],
  },
};

const BASE_URL = __ENV.O3K_URL || 'http://localhost:9696';
const KEYSTONE_URL = __ENV.KEYSTONE_URL || 'http://localhost:35357';

// Get auth token
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
  const token = getAuthToken();
  return { token };
}

export default function (data) {
  const token = data.token;
  const headers = {
    'X-Auth-Token': token,
    'Content-Type': 'application/json',
  };

  // List networks (not cached currently, but fast query)
  const listStart = Date.now();
  const listResponse = http.get(`${BASE_URL}/v2.0/networks`, { headers });
  const listDuration = Date.now() - listStart;

  const listSuccess = check(listResponse, {
    'list: status is 200': (r) => r.status === 200,
    'list: has networks': (r) => {
      const body = JSON.parse(r.body);
      return body.networks && Array.isArray(body.networks);
    },
  });

  networkListSuccessRate.add(listSuccess);
  networkListDuration.add(listDuration);

  // Get network detail (cached, 30min TTL)
  if (listSuccess && listResponse.body) {
    const body = JSON.parse(listResponse.body);
    if (body.networks && body.networks.length > 0) {
      const networkId = body.networks[0].id;

      const detailStart = Date.now();
      const detailResponse = http.get(
        `${BASE_URL}/v2.0/networks/${networkId}`,
        { headers }
      );
      const detailDuration = Date.now() - detailStart;

      const detailSuccess = check(detailResponse, {
        'detail: status is 200': (r) => r.status === 200,
        'detail: has network': (r) => {
          const body = JSON.parse(r.body);
          return body.network && body.network.id === networkId;
        },
      });

      networkDetailSuccessRate.add(detailSuccess);
      networkDetailDuration.add(detailDuration);
    }
  }

  sleep(0.1); // 100ms think time
}
