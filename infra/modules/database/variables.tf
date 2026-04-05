variable "cluster_name" {
  description = "Name prefix for the database cluster"
  type        = string
  default     = "sub2api"
}

variable "region" {
  description = "DigitalOcean region"
  type        = string
}

variable "db_size" {
  description = "Database droplet size"
  type        = string
  default     = "db-s-1vcpu-1gb"
}

variable "db_engine_version" {
  description = "PostgreSQL major version"
  type        = string
  default     = "16"
}

variable "db_name" {
  description = "Name of the database to create"
  type        = string
  default     = "sub2api"
}

variable "db_user" {
  description = "Name of the database user to create"
  type        = string
  default     = "sub2api"
}

variable "doks_cluster_id" {
  description = "DOKS cluster ID for firewall rules (restrict DB access to cluster)"
  type        = string
}

variable "tags" {
  description = "Tags to apply to database resources"
  type        = list(string)
  default     = ["sub2api", "terraform"]
}
