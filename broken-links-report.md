# Wiki Broken Links Report

**Generated:** 2025-12-12
**Pages Scanned:** ~250 (estimated 25-30% of wiki)
**Status:** Partial scan - session expired, continue in next session

---

## Truly Broken Links (404 / DNS Failure)

These links are confirmed broken and need attention:

### Azure Blob Storage (DNS Failures)

| Broken URL | Status | Found on Wiki Page |
|------------|--------|-------------------|
| https://si360helpfilesdev.blob.core.windows.net/content | 404 | https://wiki.softwareinnovation.com/wiki/360_Help |
| https://si360helpfiles.blob.core.windows.net/content | 404 | https://wiki.softwareinnovation.com/wiki/360_Help |
| https://tietoevryexpostorage.blob.core.windows.net/products/241/Documents/360%C2%B0%20Sign%20Service%20Description.pdf | DNS failure | https://wiki.softwareinnovation.com/wiki/360%C2%B0_Sign |

### GitHub Repository Links (404)

| Broken URL | Status | Found on Wiki Page |
|------------|--------|-------------------|
| https://github.com/te-industry-public360/core-360-platform/blob/master/SI.CloudOps.Scripts/Infrastructure/ArchitectureDiagrams/Azure/360-azure-architecture-vms.png | 404 | https://wiki.softwareinnovation.com/wiki/360%C2%B0_Architecture_version_6.x |
| https://github.com/te-industry-public360/core-360-platform/blob/master/SI.CloudOps.Scripts/Security/Update-CFWhitelistSourceRangeKubernetes.psm1 | 404 | https://wiki.softwareinnovation.com/wiki/360%C2%B0_Online_IP_addresses |

### Microsoft/External Documentation (404)

| Broken URL | Status | Found on Wiki Page |
|------------|--------|-------------------|
| https://devblogs.microsoft.com/premier-developer/converting-classic-azure-devops-pipelines-to-yaml/ | 404 | https://wiki.softwareinnovation.com/wiki/.NET_Core_Porting_Guide |
| https://wopi.readthedocs.io/en/latest/faq/file_sizes.html | 404 | https://wiki.softwareinnovation.com/wiki/360%C2%B0_Integration_with_MS_Office_Apps |

### Defunct Domains (DNS Failure)

| Broken URL | Status | Found on Wiki Page |
|------------|--------|-------------------|
| https://mp.software-innovation.com/Downloads | DNS failure (no such host) | https://wiki.softwareinnovation.com/wiki/360%C2%B0_Best_Practice |
| http://si-nuget-ne.cloudapp.net/nuget | DNS failure (no such host) | https://wiki.softwareinnovation.com/wiki/Asimov_Development |

### SharePoint Links (404 - Deleted/Moved)

| Broken URL | Status | Found on Wiki Page |
|------------|--------|-------------------|
| https://tietocorporation-my.sharepoint.com/:u:/r/personal/willy_koch_tieto_com/Documents/Documents/360_Online_User_Import.zip | 404 | https://wiki.softwareinnovation.com/wiki/360_Online_user_import |
| https://tietocorporation-my.sharepoint.com/:v:/g/personal/hilde_jenssen_tieto_com/EQasRTd5SUtKn0NG_FYpDNsBNSAxL0PIxZq5m5a4INFUfA | 404 | https://wiki.softwareinnovation.com/wiki/5.0_Release_information |

### Third-Party Services (404)

| Broken URL | Status | Found on Wiki Page |
|------------|--------|-------------------|
| https://www.pixedit.com/en/support/knowledge-base/ | 404 | https://wiki.softwareinnovation.com/wiki/360%C2%B0_Format_Converter |
| https://www.pixedit.com/media/1117/pixedit-list-over-all-supported-file-formats-13042018.pdf | 404 | https://wiki.softwareinnovation.com/wiki/360%C2%B0_Format_Converter |
| https://www.infotorg.no/nyheter/modernisering-av-folkeregisteret/Ofte_stilte_sp%C3%B8rsmal_om_modernisering_av_Folkeregisteret | 404 | https://wiki.softwareinnovation.com/wiki/360%C2%B0_Integration_with_Folkeregisteret |
| https://tietoevry.workplace.com/groups/607940623379933/permalink/952931772214148/ | 404 | https://wiki.softwareinnovation.com/wiki/360%C2%B0_Integration_with_Folkeregisteret |
| https://www.firstagenda.com/no/partner-login | 404 | https://wiki.softwareinnovation.com/wiki/360%C2%B0_eMeetings_Live |
| https://play.google.com/store/apps/details?id=com.softwareinnovation.emeetings | 404 (app removed) | https://wiki.softwareinnovation.com/wiki/360%C2%B0_eMeetings |

---

## Timing Out (Service May Be Down)

These URLs timed out during checking - may be temporarily unavailable or blocked:

| URL | Found on Wiki Page |
|-----|-------------------|
| https://tieto.digiforms.no/documentation | https://wiki.softwareinnovation.com/wiki/360%C2%B0_eServices_Platform |
| https://tieto-demo.digiforms.no/start | https://wiki.softwareinnovation.com/wiki/360%C2%B0_eServices_Platform |
| https://social.intra.tieto.com/tibbr/#!/messages/173024 | https://wiki.softwareinnovation.com/wiki/360%C2%B0_eMeetings |

---

## Auth Required (Expected Failures)

These return 401/403 because they require authentication. Not broken, just protected:

- All `tieto-si.visualstudio.com/*` URLs (Azure DevOps - 401)
- All `dev.azure.com/tieto-si/*` URLs (Azure DevOps - 401)
- All `tietoevry.sharepoint.com/*` URLs (SharePoint - 403)
- All `tietoevry-my.sharepoint.com/*` URLs (OneDrive - 403)
- All `tietocorporation.sharepoint.com/*` URLs (SharePoint - 403)

---

## Localhost URLs (Expected Failures)

These are development/local URLs that will never resolve externally:

| URL | Found on Wiki Page |
|-----|-------------------|
| http://localhost:81/biz/health | https://wiki.softwareinnovation.com/wiki/360_Development_Inside_Containers |
| http://localhost/sessionprops.aspx | https://wiki.softwareinnovation.com/wiki/360%C2%B0_GUI_debugging_tools_and_tips |

---

## Summary

| Category | Count |
|----------|-------|
| Truly broken (404/DNS) | 17 |
| Timing out | 3 |
| Auth required (expected) | Many |
| Localhost (expected) | 2 |

---

## Recommendations

1. **High Priority:** Fix DNS failures - these domains no longer exist
   - `mp.software-innovation.com` - old company domain
   - `si-nuget-ne.cloudapp.net` - old NuGet server
   - `tietoevryexpostorage.blob.core.windows.net` - deleted storage account

2. **Medium Priority:** Update or remove 404 links
   - GitHub repo links may have been moved/reorganized
   - SharePoint files may have been deleted
   - Third-party docs (Pixedit, Infotorg) have changed structure

3. **Low Priority:** Review timing-out URLs
   - `tieto.digiforms.no` services may need investigation

---

## Next Steps

Continue scanning remaining ~70% of wiki pages in next session to get complete coverage.

---

*Report generated by mediawiki-mcp-server link checker tool*
