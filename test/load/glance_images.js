// k6 load testing script for Glance images API
// Run: k6 run test/load/glance_images.js

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics
const imageListSuccessRate = new Rate('image_list_success_rate');
const imageDetailSuccessRate = new Rate('image_detail_success_rate');
const imageListDuration = new Trend('image_list_duration');
const imageDetailDuration = new Trend('image_detail_duration');

// Test configuration
export const options = {
  stages: [
    { duration: '30s', target: 30 },    // Ramp up to 30 req/s (baseline)
    { duration: '1m', target: 30 },     // Baseline
    { duration: '30s', target: 150 },   // Mid-range
    { duration: '1m', target: 150 },    // Sustained
    { duration: '30s', target: 300 },   // Sprint 69 target: 300 req/s
    { duration: '1m', target: 300 },    // Sustained at target
    { duration: '30s', target: 0 },     // Ramp down
  ],
  thresholds: {
    'http_req_duration': ['p(95)<5'],          // 95% under 5ms
    'image_list_success_rate': ['rate>0.99'],  // 99% success
    'image_detail_success_rate': ['rate>0.99'],
  },
};

const BASE_URL = __ENV.O3K_URL || 'http://localhost:9292';
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

  // List images (cached, 1h TTL)
  const listStart = Date.now();
  const listResponse = http.get(`${BASE_URL}/v2/images`, { headers });
  const listDuration = Date.now() - listStart;

  const listSuccess = check(listResponse, {
    'list: status is 200': (r) => r.status === 200,
    'list: has images': (r) => {
      const body = JSON.parse(r.body);
      return body.images && Array.isArray(body.images);
    },
  });

  imageListSuccessRate.add(listSuccess);
  imageListDuration.add(listDuration);

  // Get image detail (cached)
  if (listSuccess && listResponse.body) {
    const body = JSON.parse(listResponse.body);
    if (body.images && body.images.length > 0) {
      const imageId = body.images[0].id;

      const detailStart = Date.now();
      const detailResponse = http.get(
        `${BASE_URL}/v2/images/${imageId}`,
        { headers }
      );
      const detailDuration = Date.now() - detailStart;

      const detailSuccess = check(detailResponse, {
        'detail: status is 200': (r) => r.status === 200,
        'detail: has image': (r) => {
          const body = JSON.parse(r.body);
          return body.id === imageId;
        },
      });

      imageDetailSuccessRate.add(detailSuccess);
      imageDetailDuration.add(detailDuration);
    }
  }

  sleep(0.1); // 100ms think time
}
