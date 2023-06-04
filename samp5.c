#include<stdio.h>
int a = 50 ;
void fun1()
{
 int x ;
x = 100 * a ;
printf("the value of x is :%d\n",x) ;

}
void fun2()
{
int y ;
y = 1000 * a ;
printf("the value of y is : %d\n",y) ;

}
int main()
{  
 printf("the value of a is %d\n",a) ;
      fun1 () ;
 fun2 () ;
}

