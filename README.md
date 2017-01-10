Cowherd (Rancher Container SSH Utility)
===========

Native SSH Client for Rancher Containers, provided a powerful native terminal to manage your docker containers

  * It's dead simple. much like the ssh cli, you do `cowherd <environment file> container_name` to SSH into any containers
  * It's flexible. Cowherd reads configurations from ENV, from yml or json file
  * It's powerful. cowherd searches the whole rancher deployment, SSH into any containers from your workstation, regardless which host it belongs to
  * It's smart. cowherd uses fuzzy container name matching. Forget the container name? it doesn't matter, use "*" or "%" instead


Installation
============

`# go get github.com/camronlevanger/cowherd`



usage
=====

`cowherd <environment file> <container>`

Example
=======

cowherd production my-server-1
  
cowherd dev "my-server*"  (equals to) cowherd my-server%
  
cowherd staging %proxy%
  
cowherd beta "projectA-app-*" (equals to) cowherd projectA-app-%

Configuration
=============

  We read configuration from config.json or config.yml in ~, /etc/cowherd/ and ~/.cowherd/ folders.

  If you want to use JSON format, create a config.json in the folders with content:

      {
          "endpoint": "https://rancher.server/v1", // Or "https://rancher.server/v1/projects/xxxx"
          "user": "your_access_key",
          "password": "your_access_password"
      }

  If you want to use YAML format, create a config.yml with content:

      endpoint: https://your.rancher.server // Or https://rancher.server/v1/projects/xxxx
      user: your_access_key
      password: your_access_password

  We accept environment variables as well:

      SSHRANCHER_ENDPOINT=https://your.rancher.server   // Or https://rancher.server/v1/projects/xxxx
      SSHRANCHER_USER=your_access_key
      SSHRANCHER_PASSWORD=your_access_password


Flags
=====

      -h, --help     Show context-sensitive help (also try --help-long and --help-man).
      --version      Show application version.
      --endpoint=""  Rancher server endpoint, https://your.rancher.server/v1 or https://your.rancher.server/v1/projects/xxx.
      --user=""      Rancher API user/accesskey.
      --password=""  Rancher API password/secret.

**Args**

`<container>  Container name, fuzzy match`
