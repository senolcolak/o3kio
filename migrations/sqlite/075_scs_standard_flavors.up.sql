-- SCS-0103-v1 mandatory standard flavors (sqlite mirror).
-- See: https://docs.scs.community/standards/scs-0103-v1-standard-flavors/
--
-- Naming: SCS-<vCPUs><cpu-type>-<RAM_GiB>[-<disk_GB><disk-type>]
--   cpu-type: V = shared-core (default), L = low-resource
--   disk-type suffix: s = SSD; absent = no pre-attached root disk
--
-- RAM is stored in MB, so RAM_GiB * 1024.
-- is_public uses 1 (sqlite boolean) to match the existing seed pattern.
INSERT OR IGNORE INTO flavors (id, name, vcpus, ram_mb, disk_gb, is_public) VALUES
    ('00000000-0000-0000-0000-0000000005c1', 'SCS-1L-1',        1,  1024,   0, 1),
    ('00000000-0000-0000-0000-0000000005c2', 'SCS-1V-2',        1,  2048,   0, 1),
    ('00000000-0000-0000-0000-0000000005c3', 'SCS-1V-4',        1,  4096,   0, 1),
    ('00000000-0000-0000-0000-0000000005c4', 'SCS-1V-8',        1,  8192,   0, 1),
    ('00000000-0000-0000-0000-0000000005c5', 'SCS-2V-4',        2,  4096,   0, 1),
    ('00000000-0000-0000-0000-0000000005c6', 'SCS-2V-4-20s',    2,  4096,  20, 1),
    ('00000000-0000-0000-0000-0000000005c7', 'SCS-2V-8',        2,  8192,   0, 1),
    ('00000000-0000-0000-0000-0000000005c8', 'SCS-2V-16',       2, 16384,   0, 1),
    ('00000000-0000-0000-0000-0000000005c9', 'SCS-4V-8',        4,  8192,   0, 1),
    ('00000000-0000-0000-0000-0000000005ca', 'SCS-4V-16',       4, 16384,   0, 1),
    ('00000000-0000-0000-0000-0000000005cb', 'SCS-4V-16-100s',  4, 16384, 100, 1),
    ('00000000-0000-0000-0000-0000000005cc', 'SCS-4V-32',       4, 32768,   0, 1),
    ('00000000-0000-0000-0000-0000000005cd', 'SCS-8V-16',       8, 16384,   0, 1),
    ('00000000-0000-0000-0000-0000000005ce', 'SCS-8V-32',       8, 32768,   0, 1),
    ('00000000-0000-0000-0000-0000000005cf', 'SCS-16V-32',     16, 32768,   0, 1);

-- SCS metadata properties for the flavors. Operators can use these to filter
-- and map flavors to scheduling hints.
INSERT OR IGNORE INTO flavor_extra_specs (flavor_id, key, value) VALUES
    ('00000000-0000-0000-0000-0000000005c1', 'scs:cpu-type', 'crowded-core'),
    ('00000000-0000-0000-0000-0000000005c2', 'scs:cpu-type', 'shared-core'),
    ('00000000-0000-0000-0000-0000000005c3', 'scs:cpu-type', 'shared-core'),
    ('00000000-0000-0000-0000-0000000005c4', 'scs:cpu-type', 'shared-core'),
    ('00000000-0000-0000-0000-0000000005c5', 'scs:cpu-type', 'shared-core'),
    ('00000000-0000-0000-0000-0000000005c6', 'scs:cpu-type', 'shared-core'),
    ('00000000-0000-0000-0000-0000000005c6', 'scs:disk0-type', 'ssd'),
    ('00000000-0000-0000-0000-0000000005c7', 'scs:cpu-type', 'shared-core'),
    ('00000000-0000-0000-0000-0000000005c8', 'scs:cpu-type', 'shared-core'),
    ('00000000-0000-0000-0000-0000000005c9', 'scs:cpu-type', 'shared-core'),
    ('00000000-0000-0000-0000-0000000005ca', 'scs:cpu-type', 'shared-core'),
    ('00000000-0000-0000-0000-0000000005cb', 'scs:cpu-type', 'shared-core'),
    ('00000000-0000-0000-0000-0000000005cb', 'scs:disk0-type', 'ssd'),
    ('00000000-0000-0000-0000-0000000005cc', 'scs:cpu-type', 'shared-core'),
    ('00000000-0000-0000-0000-0000000005cd', 'scs:cpu-type', 'shared-core'),
    ('00000000-0000-0000-0000-0000000005ce', 'scs:cpu-type', 'shared-core'),
    ('00000000-0000-0000-0000-0000000005cf', 'scs:cpu-type', 'shared-core');
