# Golang Switch Lan Play Server

This project is a reimplementation of [spacemeowx2 Switch Lan Play server](https://github.com/spacemeowx2/switch-lan-play) made using [Golang](https://go.dev), aiming better resource saving (it runs with ~9MB of ram) and better performance.

## Usage

```sh
./switch-lan-play [options]
```

It has some options, like:
- `-port PORT` which defines the port used by the server
- `-httpAuth URL` for [HTTP authentication](#http-authentication)
- `-jsonAuth FILE` for [JSON authencticaction](#json-authentication)
- `-simpleAuth USERNAME:PASSWORD` for just simple authentication
- `-debug` show debug messages
- `-ikdebug` ignore Keepalive debug messages
- `-gnet` switches the server to GNet (improved performance and make multicore available)
- `-multicore` activates multicore *only in GNet server mode*
- `-quiet` deactivates the server monitoring message (`Client: xxx...`)
- `-pprof` activates the Golang's `pprof` tool for performance and resource profiling

## Authentication

When using [Lan Play client](https://github.com/spacemeowx2/switch-lan-play), you can pass a username and password for authentication by adding `--username USERNAME --password PASSWORD` in the client options

### HTTP Authentication

HTTP Authentication works by doing a GET request to `http://URL/?username=PASSWORD` (URL is the address passed to the option) and waits, in case of an error, for

```json
{ "error": ERROR }
```

or, in case of success, for

```json
{ "passwordSHA1": PASSWORD_SHA1 }
```

It allows the connection if `PASSWORD` SHA1 hash equals to `PASSWORD_SHA1`

### JSON Authentication

JSON Authentication works by reading a JSON file with the below structure

```json
{
    "user1": "password",
    "user2": {
        "sha1": "5baa61e4c9b93f3f0682250b6cf8331b7ee68fd8"
    }
}
```

Where `user1` and `user2` are usernames

It will only allow connection if

- (like `user1`) password equals `PASSWORD`
- (like `user2`) sha1 equals to `PASSWORD` SHA1 hash

## Building

The only build dependency is a working [Golang](https://go.dev) enviroment

```sh
# Clone the repo
git clone https://github.com/henriquetied472/golang-slp-server

# Setup dependencies
go mod tidy

# Build the binary
# Add .exe at the end for Windows build
go build -o switch-lan-play[.exe]
```