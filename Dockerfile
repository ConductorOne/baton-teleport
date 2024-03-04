FROM gcr.io/distroless/static-debian11:nonroot
ENTRYPOINT ["/baton-teleport"]
COPY baton-teleport /