-- Create databases and users for all microservices

-- User Service Database
CREATE DATABASE user_db;
CREATE USER user_service WITH PASSWORD 'user_pass';
GRANT ALL PRIVILEGES ON DATABASE user_db TO user_service;

-- Product Service Database
CREATE DATABASE product_db;
CREATE USER product_user WITH PASSWORD 'product_pass';
GRANT ALL PRIVILEGES ON DATABASE product_db TO product_user;

-- Order Service Database
CREATE DATABASE order_db;
CREATE USER order_user WITH PASSWORD 'order_pass';
GRANT ALL PRIVILEGES ON DATABASE order_db TO order_user;

-- Payment Service Database
CREATE DATABASE payment_db;
CREATE USER payment_user WITH PASSWORD 'payment_pass';
GRANT ALL PRIVILEGES ON DATABASE payment_db TO payment_user;

-- Notification Service Database
CREATE DATABASE notification_db;
CREATE USER notification_user WITH PASSWORD 'notification_pass';
GRANT ALL PRIVILEGES ON DATABASE notification_db TO notification_user;

-- Grant necessary permissions
\c user_db
GRANT ALL ON SCHEMA public TO user_service;

\c product_db
GRANT ALL ON SCHEMA public TO product_user;

\c order_db
GRANT ALL ON SCHEMA public TO order_user;

\c payment_db
GRANT ALL ON SCHEMA public TO payment_user;

\c notification_db
GRANT ALL ON SCHEMA public TO notification_user;