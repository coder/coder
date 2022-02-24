provider "google" {
  project = "coder-blacktriangle-dev"
}

resource "google_compute_instance" "dev" {
  count        = 1
  zone         = "us-central1-a"
  machine_type = "e2-medium"
  //name         = coder_workspace.self.username
  name = "bryan-test1"
  network_interface {
    network = "default"
  }
  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-9"
    }
  }
  metadata_startup_script = "echo hello, world"
}
