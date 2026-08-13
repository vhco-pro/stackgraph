terraform {
  source = "git::github.com/example/modules//app"
}

dependency "vpc" {
  config_path = "../vpc"
}
