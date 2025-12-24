# ECR Repositories for Microservices

locals {
  microservices = [
    "user-service",
    "product-service",
    "order-service",
    "payment-service",
    "notification-service"
  ]
}

# ECR Repositories
resource "aws_ecr_repository" "microservices" {
  for_each = toset(local.microservices)

  name                 = "${local.cluster_name}/${each.key}"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "AES256"
  }

  tags = merge(
    local.common_tags,
    {
      Name    = "${local.cluster_name}-${each.key}"
      Service = each.key
    }
  )
}

# ECR Lifecycle Policy
resource "aws_ecr_lifecycle_policy" "microservices" {
  for_each   = aws_ecr_repository.microservices
  repository = each.value.name

  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Keep last 10 images"
        selection = {
          tagStatus     = "tagged"
          tagPrefixList = ["v"]
          countType     = "imageCountMoreThan"
          countNumber   = 10
        }
        action = {
          type = "expire"
        }
      },
      {
        rulePriority = 2
        description  = "Remove untagged images after 7 days"
        selection = {
          tagStatus   = "untagged"
          countType   = "sinceImagePushed"
          countUnit   = "days"
          countNumber = 7
        }
        action = {
          type = "expire"
        }
      }
    ]
  })
}