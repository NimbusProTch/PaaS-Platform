# Platform Charts

This directory contains Helm charts for platform services (databases, message queues, caches, etc.).

## Structure

- `postgresql/` - PostgreSQL database chart
- `redis/` - Redis cache chart
- `rabbitmq/` - RabbitMQ message queue chart
- `mongodb/` - MongoDB database chart
- `mysql/` - MySQL database chart

These charts are embedded in the platform operator Docker image and pushed to Gitea during bootstrap.
