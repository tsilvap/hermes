# Hermes

Hermes is a data upload app.

## Usage

Create a configuration file and customize it according to your needs:

``` shell
cp config.example.toml config.toml
```

Save this file to `/etc/hermes/config.toml`. If you want to save the configuration file somewhere else, you should set the `HERMES_CONFIG` environment variable to point to its full path.

Files will be uploaded to `storage.uploaded_files_dir` (if unset, it'll default to `/var/hermes/uploaded_files/`). You'll need to create the directory beforehand.

Hermes saves the users table in a SQLite database at `storage.db_path` (if unset, it'll default to `/var/hermes/hermes.db`).

Start the server, and that's it.
