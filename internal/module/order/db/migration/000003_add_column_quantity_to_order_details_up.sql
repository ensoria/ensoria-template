-- MySQL
ALTER TABLE order_details
    ADD COLUMN quantity INT NOT NULL AFTER price;