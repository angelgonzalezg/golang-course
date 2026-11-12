// Only the explicit schema-setup Compose command runs this administrative step.
// Keep data writes with mongo_lab; dbAdmin permits collection schema changes.
const lab = db.getSiblingDB("mongo_lab");
if (lab.getUser("mongo_schema") === null) {
  lab.createUser({
    user: "mongo_schema",
    pwd: "mongo_schema_local",
    roles: [{ role: "dbAdmin", db: "mongo_lab" }],
  });
  print("Created the database-scoped schema account.");
} else {
  // Preserve existing account settings; rotating credentials is a separate action.
  print("Schema account already exists; credentials and roles were retained.");
}
