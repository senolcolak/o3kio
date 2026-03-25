-- Revert Glance endpoint URL fix
UPDATE endpoints
SET url = url || '/v2'
WHERE service_id = '00000000-0000-0000-0000-000000000014'
  AND url LIKE '%:9292'
  AND url NOT LIKE '%/v2';
