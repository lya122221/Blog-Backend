BEGIN;

LOCK TABLE users IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM users) THEN
        RAISE EXCEPTION 'legacy blog users table is not empty';
    END IF;
END;
$$;

DROP TABLE users;

COMMIT;
