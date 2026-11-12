// The official image runs this script only when initializing an empty volume.
// The application gets read/write access to this database, rather than root access.
db.getSiblingDB("mongo_lab").createUser({
  user: "mongo_lab",
  pwd: "mongo_lab_local",
  roles: [{ role: "readWrite", db: "mongo_lab" }],
});
