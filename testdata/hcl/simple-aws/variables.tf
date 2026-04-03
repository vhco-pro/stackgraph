variable "az" {
  description = "Availability zone"
  type        = string
  default     = "us-east-1a"
}

variable "ami_id" {
  description = "AMI ID for the web server"
  type        = string
  default     = "ami-0abcdef1234567890"
}
