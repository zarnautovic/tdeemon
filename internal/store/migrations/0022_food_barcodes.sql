-- A food's barcodes, each in food.Barcode's canonical form, the digits of
-- an EAN-8, an EAN-13 or a GTIN-14: a scan of one finds the food. A
-- barcode leads to one food of the user's, and a food has one for each
-- size of its package.
CREATE TABLE food_barcodes (
    food_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    code TEXT NOT NULL CHECK (
        length(code) IN (8, 13, 14) AND code NOT GLOB '*[^0-9]*'
    ),
    PRIMARY KEY (user_id, food_id, code),
    FOREIGN KEY (user_id, food_id) REFERENCES foods (user_id, id) ON DELETE CASCADE
) STRICT;

CREATE UNIQUE INDEX food_barcodes_user_code ON food_barcodes (user_id, code);
