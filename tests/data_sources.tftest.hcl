# Test: Data sources return expected data
# Run with: terraform test

run "catalog_has_packages" {
  module {
    source = "./tests/modules/data_sources"
  }

  assert {
    condition     = length(data.shc_catalog.current.packages) > 0
    error_message = "Catalog should return at least one package"
  }

  assert {
    condition     = length(data.shc_templates.available.templates) > 0
    error_message = "Templates should return at least one template"
  }
}
