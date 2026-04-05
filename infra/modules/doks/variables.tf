variable "cluster_name" {
  description = "Name of the DOKS cluster"
  type        = string
  default     = "sub2api"
}

variable "region" {
  description = "DigitalOcean region"
  type        = string
  default     = "sgp1"
}

variable "k8s_version" {
  description = "Kubernetes version prefix (latest patch auto-selected)"
  type        = string
  default     = "1.34"
}

variable "node_size" {
  description = "Droplet size for worker nodes"
  type        = string
  default     = "s-2vcpu-4gb"
}

variable "min_nodes" {
  description = "Minimum number of nodes in the autoscaling pool"
  type        = number
  default     = 1
}

variable "max_nodes" {
  description = "Maximum number of nodes in the autoscaling pool"
  type        = number
  default     = 3
}

variable "auto_upgrade" {
  description = "Enable automatic patch version upgrades"
  type        = bool
  default     = true
}

variable "surge_upgrade" {
  description = "Enable surge upgrades for zero-downtime node upgrades"
  type        = bool
  default     = true
}

variable "tags" {
  description = "Tags to apply to all resources"
  type        = list(string)
  default     = ["sub2api", "terraform"]
}
