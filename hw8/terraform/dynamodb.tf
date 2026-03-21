resource "aws_dynamodb_table" "shopping_carts" {
  name         = var.dynamodb_table_name
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "cart_id"

  attribute {
    name = "cart_id"
    type = "N"
  }

  attribute {
    name = "customer_id"
    type = "N"
  }

  attribute {
    name = "updated_at"
    type = "S"
  }

  global_secondary_index {
    name            = "customer_id-updated_at-index"
    hash_key        = "customer_id"
    range_key       = "updated_at"
    projection_type = "ALL"
  }

  tags = merge(local.common_tags, {
    Name = var.dynamodb_table_name
  })
}
