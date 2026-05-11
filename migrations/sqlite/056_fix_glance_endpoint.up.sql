-- Fix Glance endpoint URL to remove /v2 suffix
-- gophercloud's NewImageServiceV2() always appends /v2 to the catalog endpoint,
-- so we need to return just the base URL without /v2 to avoid /v2/v2 doubling

UPDATE endpoints
SET url = REPLACE(url, '/v2', '')
WHERE service_id = '00000000-0000-0000-0000-000000000014'
  AND url LIKE '%:9292/v2';
