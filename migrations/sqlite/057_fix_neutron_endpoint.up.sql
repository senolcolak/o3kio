-- Remove /v2.0 from Neutron endpoint URL to fix double path issue
-- gophercloud adds /v2.0 automatically, so the catalog shouldn't include it
UPDATE endpoints
SET url = REPLACE(url, ':9696/v2.0', ':9696')
WHERE service_id = '00000000-0000-0000-0000-000000000012' -- Neutron service
  AND url LIKE '%:9696/v2.0%';
