# Hermes

Hermes is a data upload app.

## Usage

Create a configuration file and customize it according to your needs:

``` shell
cp config.example.toml config.toml
```

Save this file with the name `config.toml` to your directory of choice. Set the `HERMES_DIR` environment variable to the name of that directory. (The default is `~/.hermes/`.)

Files will be uploaded to `storage.uploaded_files_dir` (if unset, it'll default to `$HERMES_DIR/uploaded/`). You'll need to create the directory beforehand.

Hermes saves the users table in a SQLite database at `storage.db_path` (if unset, it'll default to `$HERMES_DIR/hermes.db`).

Start the server, and that's it.
