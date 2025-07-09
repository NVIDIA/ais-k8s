This Dockerfile creates a distroless image containing only a `tail` binary. 

To use this image, attach a volume and provide the filename to `tail` to stdout.  

Example of mounting and reading from a file `./test/test.txt` in the current directory: 
```bash
docker run -v ./test:/test aistorage/ais-logs:v1.1 /test/test.txt
```

