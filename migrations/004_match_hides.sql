-- 004_match_hides.sql — viewer hides a candidate from future match lists

CREATE TABLE match_hides (
    viewer_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    candidate_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    hidden_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (viewer_id, candidate_id),
    CHECK (viewer_id <> candidate_id)
);

CREATE INDEX idx_match_hides_viewer ON match_hides (viewer_id);
