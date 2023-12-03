import { Component, OnInit } from '@angular/core';
import { User } from '../model/user';
import { ProfileService } from '../services/profile.service';
import { AuthService } from '../services/auth.service';
import { ToastrService } from 'ngx-toastr';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';

@Component({
  selector: 'app-profile-details',
  templateUrl: './profile-details.component.html',
  styleUrls: ['./profile-details.component.css']
})
export class ProfileDetailsComponent implements OnInit {
  form!: FormGroup;
  userProfile?: User;
  username: string = this.authService.getUsernameFromToken() || '';
  updateSuccess: boolean = false;

  constructor(private profileService: ProfileService, private authService: AuthService, private toastr: ToastrService, private fb: FormBuilder) {}

  ngOnInit(): void {
    this.loadUserProfile();

    this.form = this.fb.group({
      username: [null, Validators.required],
      firstName: [null, Validators.required],
      lastName: [null, Validators.required],
      email: [null, Validators.email],
      address: [null, Validators.required],
    });
  }

  loadUserProfile() {
    this.profileService.getUser(this.username).subscribe(
      (data) => {
        this.userProfile = data;
      },
      (error) => {
        console.error('Error fetching user profile: ', error);
      }
    );
  }

  updateProfile() {
    if (this.form.invalid) {
      this.toastr.warning('Please fill in the required fields correctly', 'Form Validation');
      return;
    }

    const usernameRegex: RegExp = /^(?=.{3,20}$)(?![_.])(?!.*[_.]{2})[a-zA-Z0-9._]+(?<![_.])$/;
    const nameRegex: RegExp = /^(?=.{1,35}$)[A-Za-z]+(?:[' -][A-Za-z]+)*$/;
    const addressRegex: RegExp = /^[A-Za-z0-9](?!.*['\.\-\s\,]$)[A-Za-z0-9'\.\-\s\,]{0,68}[A-Za-z0-9]$/;

    if (!usernameRegex.test(this.form.value.username)) {
      this.toastr.warning(' Alphanumeric characters, underscore and dot are allowed. Min. length 3, max. length 20.' +
      ' Special characters can`t be next to each other',
        'Invalid username');
        return;
    }

    if (!nameRegex.test(this.form.value.firstName) || !nameRegex.test(this.form.value.lastName)) {
      this.toastr.warning('First Name and/or Last Name are not valid inputs', 'Invalid personal detail');
      return;
    }

    if (!addressRegex.test(this.form.value.address)) {
      this.toastr.warning('Address format is not valid', 'Invalid address');
      return;
    }

    if (this.userProfile) {
      // Update userProfile with form values
      this.userProfile.username = this.form.value.username;
      this.userProfile.firstName = this.form.value.firstName;
      this.userProfile.lastName = this.form.value.lastName;
      this.userProfile.email = this.form.value.email;
      this.userProfile.address = this.form.value.address;

      this.profileService.updateUser(this.username, this.userProfile).subscribe(
        () => {
          if (this.username === this.userProfile?.username) {
            this.toastr.success('Profile updated successfully', 'Profile update');
            this.loadUserProfile();
          } else {
            this.toastr.success('Profile updated successfully. Relogin with new username',
             'Profile update');
            this.authService.logout();
          }
        },
        (error) => {
          console.error('Error updating user profile: ', error);
          this.toastr.error('Update profile failed.', 'Failed update');
        }
      );
    } else {
      console.error('User profile is undefined. Cannot update.');
    }
  }
}
