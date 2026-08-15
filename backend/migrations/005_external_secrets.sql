-- Connection strings and signing keys are deployment secrets, not application data.
DELETE FROM config WHERE name IN ('DATABASE_URL', 'NATS_URL', 'CONTROL_PLANE_SIGNING_PRIVATE_KEY');
