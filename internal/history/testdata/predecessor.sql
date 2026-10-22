-- The predecessor's tables of days and meals, as its index of the vault's
-- daily notes made them (health-api, internal/store), for tests: the rows
-- each test adds are made up.
CREATE TABLE days (
    "date" TEXT PRIMARY KEY,
    kalorije REAL, proteini REAL, ugljikohidrati REAL, masti REAL,
    kilaza REAL, koraci INTEGER, aktivne_kalorije REAL, bazalne_kalorije REAL,
    san REAL, rhr INTEGER, hrv REAL, vo2max REAL, masnoca REAL,
    misicna_masa REAL,
    trening_min INTEGER, osjecaj INTEGER, glad INTEGER,
    status TEXT, treninzi TEXT, napomena TEXT
);
CREATE TABLE meals (
    "date" TEXT NOT NULL,
    "time" TEXT,
    description TEXT,
    kcal REAL, proteini REAL, ugljikohidrati REAL, masti REAL
);
