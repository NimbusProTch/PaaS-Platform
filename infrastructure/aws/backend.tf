terraform {
  backend "local" {
    path = "terraform.tfstate"
  }
}

# For production, use S3 backend:
# terraform {
#   backend "s3" {
#     bucket         = "infraforge-terraform-state"
#     key            = "eks/terraform.tfstate"
#     region         = "eu-west-1"
#     encrypt        = true
#     dynamodb_table = "terraform-state-lock"
#   }
# }