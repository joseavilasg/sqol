package main

import (
	"strings"
	"testing"
)

func TestFormatPutsTheAndOneLevelUnderItsClause(t *testing.T) {
	in := "" +
		"SELECT id FROM library.books\n" +
		"WHERE shelf_id IS NOT DISTINCT FROM $1\n" +
		"  AND member_id = $2\n" +
		"  AND state = 'available';\n"

	want := "" +
		"SELECT id FROM library.books\n" +
		"WHERE shelf_id IS NOT DISTINCT FROM $1\n" +
		"\tAND member_id = $2\n" +
		"\tAND state = 'available';\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatPutsOnAndItsAndBelowTheJoin(t *testing.T) {
	in := "" +
		"FROM library.book_tags AS bt\n" +
		"    LEFT JOIN library.tags AS t\n" +
		"        ON t.name = bt.tag_name\n" +
		"        AND t.state = 'live'\n" +
		"    JOIN library.books AS b ON b.id = bt.book_id;\n"

	want := "" +
		"FROM library.book_tags AS bt\n" +
		"\tLEFT JOIN library.tags AS t\n" +
		"\t\tON t.name = bt.tag_name\n" +
		"\t\tAND t.state = 'live'\n" +
		"\tJOIN library.books AS b ON b.id = bt.book_id;\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatFollowsTheBlocksOfAProceduralBody(t *testing.T) {
	in := "" +
		"DO $$\n" +
		"BEGIN\n" +
		"    IF EXISTS (\n" +
		"        SELECT 1 FROM information_schema.columns\n" +
		"        WHERE table_schema = 'library' AND table_name = 'loans'\n" +
		"        AND column_name = 'member_id'\n" +
		"    ) THEN\n" +
		"        ALTER TABLE library.loans ALTER COLUMN member_id SET NOT NULL;\n" +
		"    ELSE\n" +
		"        RAISE NOTICE 'nothing to do';\n" +
		"    END IF;\n" +
		"END $$;\n"

	want := "" +
		"DO $$\n" +
		"BEGIN\n" +
		"\tIF EXISTS (\n" +
		"\t\tSELECT 1 FROM information_schema.columns\n" +
		"\t\tWHERE table_schema = 'library' AND table_name = 'loans'\n" +
		"\t\t\tAND column_name = 'member_id'\n" +
		"\t) THEN\n" +
		"\t\tALTER TABLE library.loans ALTER COLUMN member_id SET NOT NULL;\n" +
		"\tELSE\n" +
		"\t\tRAISE NOTICE 'nothing to do';\n" +
		"\tEND IF;\n" +
		"END $$;\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatLeavesASettledFileAlone(t *testing.T) {
	in := "" +
		"SELECT\n" +
		"\tloan_id, code, member_id, shelf_name\n" +
		"FROM library.loans\n" +
		"WHERE member_id = $1\n" +
		"\tAND code <> ''\n" +
		"ORDER BY code;\n"

	once := format(in)
	if once != in {
		t.Fatalf("a settled file changed:\n%s", arrow(once))
	}
	if twice := format(once); twice != once {
		t.Errorf("the second pass changed the text:\n%s", arrow(twice))
	}
}

func TestFormatDoesNotReadInsideLiteralsOrComments(t *testing.T) {
	in := "-- AND is not a clause here\n" +
		"SELECT 'WHERE -- AND' AS note\n" +
		"FROM library.books\n" +
		"WHERE note <> 'x';\n"

	got := format(in)
	if !strings.Contains(got, "-- AND is not a clause here\n") {
		t.Errorf("the comment moved:\n%s", arrow(got))
	}
	if !strings.Contains(got, "SELECT 'WHERE -- AND' AS note\n") {
		t.Errorf("the literal moved:\n%s", arrow(got))
	}
}

// arrow renders tabs so a diff in a failure reads as indentation and not as a
// wall of blank space.
func arrow(text string) string {
	return strings.ReplaceAll(text, "\t", "→")
}

func TestFormatKeepsACommentWithTheBlockItExplains(t *testing.T) {
	in := "" +
		"CREATE TABLE library.books (\n" +
		"    id text NOT NULL,\n" +
		"    -- The catalogue publishes this as an enum, so a value outside it is a\n" +
		"    -- bug worth refusing rather than storing.\n" +
		"    kind text NOT NULL,\n" +
		"    CONSTRAINT books_kind_check CHECK (kind IN ('book', 'box'))\n" +
		");\n"

	want := "" +
		"CREATE TABLE library.books (\n" +
		"\tid   TEXT NOT NULL,\n" +
		"\t-- The catalogue publishes this as an enum, so a value outside it is a\n" +
		"\t-- bug worth refusing rather than storing.\n" +
		"\tkind TEXT NOT NULL,\n" +
		"\tCONSTRAINT books_kind_check CHECK (kind IN ('book', 'box'))\n" +
		");\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatKeepsACommentBetweenListItems(t *testing.T) {
	in := "" +
		"SELECT\n" +
		"\ta,\n" +
		"\t-- the second column exists for the client\n" +
		"\tb\n" +
		"FROM library.books;\n"

	want := "" +
		"SELECT\n" +
		"\ta,\n" +
		"\t-- the second column exists for the client\n" +
		"\tb\n" +
		"FROM library.books;\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatIndentsAContinuedStatementOnce(t *testing.T) {
	in := "" +
		"ALTER TABLE library.loans\n" +
		"ADD CONSTRAINT loans_member_id_members_id_fk\n" +
		"FOREIGN KEY (member_id) REFERENCES library.members (id);\n"

	want := "" +
		"ALTER TABLE library.loans\n" +
		"\tADD CONSTRAINT loans_member_id_members_id_fk\n" +
		"\tFOREIGN KEY (member_id) REFERENCES library.members (id);\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatIndentsTheListOfANestedSelect(t *testing.T) {
	in := "" +
		"FROM (\n" +
		"    SELECT\n" +
		"    bt.book_id,\n" +
		"    jsonb_agg(t.name ORDER BY bt.ord) AS tags\n" +
		"    FROM library.book_tags AS bt\n" +
		") AS tagged\n"

	want := "" +
		"FROM (\n" +
		"\tSELECT\n" +
		"\t\tbt.book_id,\n" +
		"\t\tJSONB_AGG(t.name ORDER BY bt.ord) AS tags\n" +
		"\tFROM library.book_tags AS bt\n" +
		") AS tagged\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatIndentsConditionsInsideAClauseParenthesis(t *testing.T) {
	in := "" +
		"WHERE shelf_id IS NOT DISTINCT FROM $1\n" +
		"    AND member_id = $2\n" +
		"    AND (\n" +
		"        a = 'asc'\n" +
		"    OR b = 'desc'\n" +
		"    )\n" +
		"ORDER BY\n" +
		"    title ASC\n"

	want := "" +
		"WHERE shelf_id IS NOT DISTINCT FROM $1\n" +
		"\tAND member_id = $2\n" +
		"\tAND (\n" +
		"\t\ta = 'asc'\n" +
		"\t\tOR b = 'desc'\n" +
		"\t)\n" +
		"ORDER BY\n" +
		"\ttitle ASC\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatKeepsADeclarationListFlat(t *testing.T) {
	in := "" +
		"DO $$\n" +
		"DECLARE\n" +
		"    a bigint;\n" +
		"    b bigint;\n" +
		"    c bigint;\n" +
		"BEGIN\n" +
		"    SELECT 1;\n" +
		"END $$;\n"

	want := "" +
		"DO $$\n" +
		"DECLARE\n" +
		"\ta BIGINT;\n" +
		"\tb BIGINT;\n" +
		"\tc BIGINT;\n" +
		"BEGIN\n" +
		"\tSELECT 1;\n" +
		"END $$;\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatStartsAStatementAfterASemicolon(t *testing.T) {
	in := "" +
		"ALTER TABLE library.books\n" +
		"ADD CONSTRAINT a CHECK (x);\n" +
		"ALTER TABLE library.books\n" +
		"ADD CONSTRAINT b CHECK (y);\n"

	want := "" +
		"ALTER TABLE library.books\n" +
		"\tADD CONSTRAINT a CHECK (x);\n" +
		"ALTER TABLE library.books\n" +
		"\tADD CONSTRAINT b CHECK (y);\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatAlinesTheTermsOfAnExpression(t *testing.T) {
	in := "" +
		"WHERE COALESCE(total_pages, 0)\n" +
		"    + COALESCE((\n" +
		"        SELECT SUM(l.pages)\n" +
		"        FROM library.loans AS l\n" +
		"        WHERE l.member_id = m.id\n" +
		"    ), 0)\n" +
		"    + $1::bigint <= max_pages;\n"

	// The shape of the whole expression is settled, not the shape of one pass:
	// hanging a parenthesis costs a pass per level, so the command applies the
	// rewrite until the text stops moving.
	settled, err := settle(in)
	if err != nil {
		t.Fatalf("settle: %v", err)
	}

	want := "" +
		"WHERE COALESCE(total_pages, 0)\n" +
		"\t+ COALESCE(\n" +
		"\t\t(\n" +
		"\t\t\tSELECT SUM(l.pages)\n" +
		"\t\t\tFROM library.loans AS l\n" +
		"\t\t\tWHERE l.member_id = m.id\n" +
		"\t\t),\n" +
		"\t\t0\n" +
		"\t)\n" +
		"\t+ $1::BIGINT <= max_pages;\n"

	if settled != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(settled), arrow(want))
	}
}

func TestFormatFollowsAForLoopHeader(t *testing.T) {
	in := "" +
		"DO $$\n" +
		"BEGIN\n" +
		"    FOR shelf IN SELECT id FROM library.shelves LOOP\n" +
		"    SELECT COUNT(*) INTO total FROM library.books;\n" +
		"    END LOOP;\n" +
		"END $$;\n"

	want := "" +
		"DO $$\n" +
		"BEGIN\n" +
		"\tFOR shelf IN SELECT id FROM library.shelves LOOP\n" +
		"\t\tSELECT COUNT(*) INTO total FROM library.books;\n" +
		"\tEND LOOP;\n" +
		"END $$;\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatKeepsTheNamesOfACteListAtOneLevel(t *testing.T) {
	in := "" +
		"WITH all_loans AS (\n" +
		"    SELECT loan_id FROM library.loans\n" +
		"),\n" +
		"tags AS (\n" +
		"    SELECT name FROM library.tags\n" +
		")\n" +
		"SELECT * FROM all_loans;\n"

	want := "" +
		"WITH all_loans AS (\n" +
		"\tSELECT loan_id FROM library.loans\n" +
		"),\n" +
		"tags AS (\n" +
		"\tSELECT name FROM library.tags\n" +
		")\n" +
		"SELECT * FROM all_loans;\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatIndentsTheAssignmentsOfASet(t *testing.T) {
	in := "" +
		"UPDATE library.members\n" +
		"SET username = $1,\n" +
		"password_hash = $2,\n" +
		"verified = FALSE\n" +
		"WHERE id = $3;\n"

	want := "" +
		"UPDATE library.members\n" +
		"SET username = $1,\n" +
		"\tpassword_hash = $2,\n" +
		"\tverified = FALSE\n" +
		"WHERE id = $3;\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatLinesUpTheColumnsOfATable(t *testing.T) {
	in := "" +
		"CREATE TABLE library.members (\n" +
		"id                    text                     NOT NULL,\n" +
		"\tmember_no        bigint                   NOT NULL,\n" +
		"\tusername              text                     NOT NULL,\n" +
		"\tpassword_hash text                    NOT NULL,\n" +
		"\tdefault_shelf     text                     NOT NULL DEFAULT '',\n" +
		"\ttotal_pages          bigint                            DEFAULT 0,\n" +
		"\tmax_pages           bigint                            DEFAULT 1000000000000,\n" +
		"\tis_active             boolean                          DEFAULT TRUE,\n" +
		"\tverified          boolean                          DEFAULT FALSE,\n" +
		"\tcreated_at            timestamp                NOT NULL DEFAULT (NOW() AT TIME ZONE 'utc'),\n" +
		"\tCONSTRAINT members_pkey PRIMARY KEY (id)\n" +
		");\n"

	want := "" +
		"CREATE TABLE library.members (\n" +
		"\tid            TEXT      NOT NULL,\n" +
		"\tmember_no     BIGINT    NOT NULL,\n" +
		"\tusername      TEXT      NOT NULL,\n" +
		"\tpassword_hash TEXT      NOT NULL,\n" +
		"\tdefault_shelf TEXT      NOT NULL DEFAULT '',\n" +
		"\ttotal_pages   BIGINT             DEFAULT 0,\n" +
		"\tmax_pages     BIGINT             DEFAULT 1000000000000,\n" +
		"\tis_active     BOOLEAN            DEFAULT TRUE,\n" +
		"\tverified      BOOLEAN            DEFAULT FALSE,\n" +
		"\tcreated_at    TIMESTAMP NOT NULL DEFAULT (NOW() AT TIME ZONE 'utc'),\n" +
		"\tCONSTRAINT members_pkey PRIMARY KEY (id)\n" +
		");\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
	if twice := format(want); twice != want {
		t.Errorf("the second pass changed the text:\n%s", arrow(twice))
	}
}

func TestFormatPutsAJoinBelowItsFrom(t *testing.T) {
	in := "" +
		"FROM library.books AS b\n" +
		"JOIN library.tags AS t ON t.loan_id = b.loan_id\n" +
		"LEFT JOIN library.book_tags AS bt\n" +
		"ON bt.tag_name = t.name\n" +
		"AND bt.book_id = b.id\n" +
		"WHERE b.state = 'available';\n"

	want := "" +
		"FROM library.books AS b\n" +
		"\tJOIN library.tags AS t ON t.loan_id = b.loan_id\n" +
		"\tLEFT JOIN library.book_tags AS bt\n" +
		"\t\tON bt.tag_name = t.name\n" +
		"\t\tAND bt.book_id = b.id\n" +
		"WHERE b.state = 'available';\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatSurroundsAUnionWithBlankLines(t *testing.T) {
	in := "" +
		"SELECT a FROM t1\n" +
		"UNION ALL\n" +
		"SELECT a FROM t2;\n"

	want := "" +
		"SELECT a FROM t1\n" +
		"\n" +
		"UNION ALL\n" +
		"\n" +
		"SELECT a FROM t2;\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
	if twice := format(want); twice != want {
		t.Errorf("the second pass changed the text:\n%s", arrow(twice))
	}
}

func TestFormatSplitsAUnionThatSharesItsLine(t *testing.T) {
	in := "" +
		"SELECT *\n" +
		"FROM (SELECT * FROM a UNION ALL SELECT * FROM b) AS u\n"

	want := "" +
		"SELECT *\n" +
		"FROM (\n" +
		"\tSELECT * FROM a\n" +
		"\n" +
		"\tUNION ALL\n" +
		"\n" +
		"\tSELECT * FROM b\n" +
		") AS u\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
	// The keyword only learns it sits inside the parenthesis on the next pass,
	// so the settled text must not change again.
	settled, err := settle(in)
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if again := format(settled); again != settled {
		t.Errorf("the settled text changed again:\n%s", arrow(again))
	}
}

func TestFormatKeepsTheOnOfAnIndexOnOneLevel(t *testing.T) {
	in := "" +
		"CREATE UNIQUE INDEX members_identity_shelf_key\n" +
		"ON library.members (member_no, default_shelf);\n" +
		"CREATE INDEX books_shelf_idx\n" +
		"    ON library.books (shelf_id)\n" +
		"    WHERE shelf_id IS NOT NULL;\n"

	want := "" +
		"CREATE UNIQUE INDEX members_identity_shelf_key\n" +
		"\tON library.members (member_no, default_shelf);\n" +
		"CREATE INDEX books_shelf_idx\n" +
		"\tON library.books (shelf_id)\n" +
		"WHERE shelf_id IS NOT NULL;\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatPutsTheClosingParenOfAGroupOnItsOwnLine(t *testing.T) {
	in := "" +
		"SELECT u.total\n" +
		"FROM (SELECT * FROM all_loans\n" +
		"\n" +
		"\tUNION ALL\n" +
		"\n" +
		"\tSELECT * FROM tags) AS u\n" +
		"GROUP BY u.total;\n"

	want := "" +
		"SELECT u.total\n" +
		"FROM (\n" +
		"\tSELECT * FROM all_loans\n" +
		"\n" +
		"\tUNION ALL\n" +
		"\n" +
		"\tSELECT * FROM tags\n" +
		") AS u\n" +
		"GROUP BY u.total;\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatUppercasesKeywordsOnly(t *testing.T) {
	in := "" +
		"-- select from where stays as written inside a comment\n" +
		"select b.kind, b.title, 'select from' as note\n" +
		"from library.books as b\n" +
		"where b.kind = 'box' and b.pages > 0\n" +
		"order by b.title desc;\n"

	want := "" +
		"-- select from where stays as written inside a comment\n" +
		"SELECT b.kind, b.title, 'select from' AS note\n" +
		"FROM library.books AS b\n" +
		"WHERE b.kind = 'box' AND b.pages > 0\n" +
		"ORDER BY b.title DESC;\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}

func TestFormatUppercasesBuiltInFunctions(t *testing.T) {
	in := "" +
		"select substring(title from 1 for 3), count(*), jsonb_agg(t.name)\n" +
		"from library.tags t\n" +
		"where coalesce(t.size, 0) > 0;\n"

	want := "" +
		"SELECT SUBSTRING(title FROM 1 FOR 3), COUNT(*), JSONB_AGG(t.name)\n" +
		"FROM library.tags t\n" +
		"WHERE COALESCE(t.size, 0) > 0;\n"

	if got := format(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", arrow(got), arrow(want))
	}
}
