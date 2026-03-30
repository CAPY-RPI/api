CREATE VIEW events_with_org_ids AS
    SELECT
        e.*,
        ARRAY_AGG(eh.oid)::uuid[] AS org_ids
    FROM events e
    LEFT JOIN event_hosting eh ON e.eid = eh.eid
    GROUP BY
        e.eid;
