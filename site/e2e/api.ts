import type { Page } from "@playwright/test";
import * as API from "api/api";
import { coderPort } from "./constants";
import { findSessionToken, randomName } from "./helpers";

let currentOrgId: string;

export const setupApiCalls = async (page: Page) => {
  const token = await findSessionToken(page);
  API.setSessionToken(token);
  API.setHost(`http://127.0.0.1:${coderPort}`);
};

export const getCurrentOrgId = async (): Promise<string> => {
  if (currentOrgId) {
    return currentOrgId;
  }
  const currentUser = await API.getAuthenticatedUser();
  currentOrgId = currentUser.organization_ids[0];
  return currentOrgId;
};

export const createUser = async (orgId: string) => {
  const name = randomName();
  const user = await API.createUser({
    email: `${name}@coder.com`,
    username: name,
    password: "s3cure&password!",
    login_type: "password",
    disable_login: false,
    organization_id: orgId,
  });
  return user;
};

export const createGroup = async (orgId: string) => {
  const name = randomName();
  const group = await API.createGroup(orgId, {
    name,
    display_name: `Display ${name}`,
    avatar_url: "/emojis/1f60d.png",
    quota_allowance: 0,
  });
  return group;
};
