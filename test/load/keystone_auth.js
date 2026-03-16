// k6 load testing script for O3K performance benchmarks
// Run: k6 run test/load/keystone_auth.js

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics
const authSuccessRate = new Rate('auth_success_rate');
const authDuration = new Trend('auth_duration');
const catalogSize = new Trend('catalog_size');

// Test configuration
export const options = {
  stages: [
    { duration: '30s', target: 100 },   // Ramp up to 100 users
    { duration: '1m', target: 100 },    // Stay at 100 users
    { duration: '30s', target: 500 },   // Ramp up to 500 users
    { duration: '1m', target: 500 },    // Stay at 500 users
    { duration: '30s', target: 1000 },  // Ramp up to 1000 users (Sprint 69 target)
    { duration: '1m', target: 1000 },   // Stay at 1000 users
    { duration: '30s', target: 0 },     // Ramp down
  ],
  thresholds: {
    'http_req_duration': ['p(95)<5'],     // 95% of requests under 5ms (Sprint 69 target)
    'auth_success_rate': ['rate>0.99'],   // 99% success rate
    'http_req_failed': ['rate<0.01'],     // Less than 1% failures
  },
};

const BASE_URL = __ENV.O3K_URL || 'http://localhost:35357';

export default function () {
  // Authenticate with password
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

  const params = {
    headers: {
      'Content-Type': 'application/json',
    },
  };

  const startTime = Date.now();
  const authResponse = http.post(
    `${BASE_URL}/v3/auth/tokens`,
    authPayload,
    params
  );
  const duration = Date.now() - startTime;

  // Check response
  const success = check(authResponse, {
    'status is 201': (r) => r.status === 201,
    'has token header': (r) => r.headers['X-Subject-Token'] !== undefined,
    'has catalog': (r) => {
      const body = JSON.parse(r.body);
      return body.token && body.token.catalog && body.token.catalog.length > 0;
    },
  });

  // Record metrics
  authSuccessRate.add(success);
  authDuration.add(duration);

  if (success && authResponse.body) {
    const body = JSON.parse(authResponse.body);
    if (body.token && body.token.catalog) {
      catalogSize.add(body.token.catalog.length);
    }
  }

  sleep(0.1); // 100ms think time between requests
}

// Smoke test scenario - minimal load to verify setup
export function smokeTest() {
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
    },
  });

  const authResponse = http.post(
    `${BASE_URL}/v3/auth/tokens`,
    authPayload,
    { headers: { 'Content-Type': 'application/json' } }
  );

  check(authResponse, {
    'smoke: status is 201': (r) => r.status === 201,
    'smoke: has token': (r) => r.headers['X-Subject-Token'] !== undefined,
  });
}
