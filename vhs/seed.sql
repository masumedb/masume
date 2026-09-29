-- The database the demo recording reads. `mise run demo` loads it into the postgres
-- container from compose.yaml, so the GIF can be rebuilt from nothing.
DROP VIEW IF EXISTS order_totals;
DROP TABLE IF EXISTS order_items, orders, customers;

CREATE TABLE customers (
  id         serial PRIMARY KEY,
  name       text NOT NULL,
  email      text UNIQUE NOT NULL,
  country    text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE orders (
  id          serial PRIMARY KEY,
  customer_id integer NOT NULL REFERENCES customers(id),
  status      text NOT NULL,
  total       numeric(10,2) NOT NULL,
  placed_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX orders_customer_id_idx ON orders (customer_id);
CREATE INDEX orders_status_idx ON orders (status);

CREATE TABLE order_items (
  id       serial PRIMARY KEY,
  order_id integer NOT NULL REFERENCES orders(id),
  sku      text NOT NULL,
  qty      integer NOT NULL,
  price    numeric(10,2) NOT NULL
);

CREATE VIEW order_totals AS
  SELECT o.id, o.status, sum(i.qty * i.price) AS total
  FROM orders o JOIN order_items i ON i.order_id = o.id
  GROUP BY o.id, o.status;

INSERT INTO customers (name, email, country, created_at)
SELECT 'Customer ' || g, 'user' || g || '@example.com',
       CASE WHEN g * 7 % 100 < 30 THEN 'DE'
            WHEN g * 7 % 100 < 52 THEN 'GB'
            WHEN g * 7 % 100 < 70 THEN 'FR'
            WHEN g * 7 % 100 < 84 THEN 'NL'
            WHEN g * 7 % 100 < 94 THEN 'ES'
            ELSE 'PL' END,
       now() - make_interval(days => 400 + g % 700)
FROM generate_series(1, 500) g;

INSERT INTO orders (customer_id, status, total, placed_at)
SELECT 1 + (g % 500),
       CASE WHEN g * 13 % 100 < 45 THEN 'shipped'
            WHEN g * 13 % 100 < 75 THEN 'paid'
            WHEN g * 13 % 100 < 90 THEN 'new'
            ELSE 'refunded' END,
       round((random() * 400 + 10)::numeric, 2),
       date_trunc('second', now() - make_interval(days => g * 37 % 365, hours => g % 24,
                                                  mins => g * 11 % 60))
FROM generate_series(1, 4000) g;

INSERT INTO order_items (order_id, sku, qty, price)
SELECT 1 + (g % 4000), 'SKU-' || lpad(((g % 250) + 1)::text, 4, '0'),
       1 + (g % 5), round((random() * 90 + 5)::numeric, 2)
FROM generate_series(1, 12000) g;

ANALYZE;
