-- +goose Up
CREATE TABLE system.resource_revisions (
    scope VARCHAR(40) NOT NULL,
    resource_id UUID NOT NULL,
    revision BIGINT NOT NULL,
    PRIMARY KEY (scope, resource_id)
);

-- +goose Down
DROP TABLE system.resource_revisions;
